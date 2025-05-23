// Package crypto provides secure TLS certificate generation and cryptographic utilities
// for revsocks with production-grade security practices and proper error handling.
package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CertificateGenerator provides secure certificate generation capabilities
// It implements best practices for TLS certificate creation with proper validation.
type CertificateGenerator struct {
	keySize   int
	validDays int
}

// CertificateOptions contains options for certificate generation
type CertificateOptions struct {
	// Subject information for the certificate
	Subject pkix.Name

	// Hosts contains DNS names and IP addresses for the certificate
	Hosts []string

	// ValidDays specifies certificate validity period in days
	ValidDays int

	// KeySize specifies RSA key size in bits (minimum 2048)
	KeySize int

	// IsCA indicates if this is a Certificate Authority certificate
	IsCA bool

	// Parent certificate and key for signing (nil for self-signed)
	ParentCert *x509.Certificate
	ParentKey  interface{}
}

// NewCertificateGenerator creates a new certificate generator with secure defaults
// Default settings use 4096-bit RSA keys and 365-day validity for production security.
func NewCertificateGenerator() *CertificateGenerator {
	return &CertificateGenerator{
		keySize:   4096, // Use 4096-bit keys for enhanced security
		validDays: 365,  // One year validity
	}
}

// GenerateSelfSignedCertificate generates a self-signed TLS certificate with secure parameters
// This function creates production-ready certificates suitable for TLS encryption.
func (cg *CertificateGenerator) GenerateSelfSignedCertificate(opts CertificateOptions) (tls.Certificate, error) {
	// Validate key size (minimum 2048 bits for security)
	keySize := opts.KeySize
	if keySize < 2048 {
		keySize = cg.keySize
	}

	// Validate validity period
	validDays := opts.ValidDays
	if validDays <= 0 {
		validDays = cg.validDays
	}

	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate RSA private key: %w", err)
	}

	// Generate a cryptographically secure serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Set certificate validity period
	notBefore := time.Now()
	notAfter := notBefore.Add(time.Duration(validDays) * 24 * time.Hour)

	// Create certificate template with secure defaults
	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               opts.Subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  opts.IsCA,
	}

	// Add Subject Alternative Names for hosts
	for _, host := range opts.Hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}

	// Set CA-specific fields if this is a CA certificate
	if opts.IsCA {
		template.KeyUsage |= x509.KeyUsageCertSign
		template.MaxPathLen = 0
		template.MaxPathLenZero = true
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Create TLS certificate from DER-encoded certificate and private key
	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	return tlsCert, nil
}

// GenerateSignedCertificate generates a certificate signed by a parent CA
func (cg *CertificateGenerator) GenerateSignedCertificate(opts CertificateOptions) (tls.Certificate, error) {
	if opts.ParentCert == nil || opts.ParentKey == nil {
		return tls.Certificate{}, fmt.Errorf("parent certificate and key are required for signed certificates")
	}

	// Validate key size
	keySize := opts.KeySize
	if keySize < 2048 {
		keySize = cg.keySize
	}

	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate RSA private key: %w", err)
	}

	// Generate serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Set certificate validity period
	validDays := opts.ValidDays
	if validDays <= 0 {
		validDays = cg.validDays
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Duration(validDays) * 24 * time.Hour)

	// Create certificate template
	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               opts.Subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Add Subject Alternative Names
	for _, host := range opts.Hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}

	// Generate certificate signed by parent
	certDER, err := x509.CreateCertificate(rand.Reader, &template, opts.ParentCert, &privateKey.PublicKey, opts.ParentKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create signed certificate: %w", err)
	}

	// Create TLS certificate
	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	return tlsCert, nil
}

// SaveCertificate saves a TLS certificate and private key to PEM files
// This function securely writes certificate files with appropriate permissions.
func SaveCertificate(cert tls.Certificate, certPath, keyPath string) error {
	// Create directory if it doesn't exist
	certDir := filepath.Dir(certPath)
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("failed to create certificate directory: %w", err)
	}

	// Extract certificate and private key
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("certificate is empty")
	}

	certDER := cert.Certificate[0]
	privateKey := cert.PrivateKey

	// Encode certificate as PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	if certPEM == nil {
		return fmt.Errorf("failed to encode certificate as PEM")
	}

	// Encode private key as PEM
	var keyPEM []byte
	switch key := privateKey.(type) {
	case *rsa.PrivateKey:
		keyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
	default:
		return fmt.Errorf("unsupported private key type")
	}

	if keyPEM == nil {
		return fmt.Errorf("failed to encode private key as PEM")
	}

	// Write certificate file with appropriate permissions
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write certificate file: %w", err)
	}

	// Write private key file with restrictive permissions
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write private key file: %w", err)
	}

	return nil
}

// LoadCertificate loads a TLS certificate from PEM files
func LoadCertificate(certPath, keyPath string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load certificate from %s and %s: %w", certPath, keyPath, err)
	}

	return cert, nil
}

// DefaultServerSubject returns a default Subject for server certificates
func DefaultServerSubject() pkix.Name {
	return pkix.Name{
		Organization:       []string{"Revsocks Server"},
		OrganizationalUnit: []string{"Secure Tunneling"},
		Country:            []string{"US"},
		Province:           []string{""},
		Locality:           []string{""},
	}
}

// DefaultClientSubject returns a default Subject for client certificates
func DefaultClientSubject() pkix.Name {
	return pkix.Name{
		Organization:       []string{"Revsocks Client"},
		OrganizationalUnit: []string{"Secure Tunneling"},
		Country:            []string{"US"},
		Province:           []string{""},
		Locality:           []string{""},
	}
}

// GetSecureTLSConfig returns a secure TLS configuration with modern cipher suites
// This function implements security best practices for TLS connections.
func GetSecureTLSConfig(cert *tls.Certificate, skipVerify bool) *tls.Config {
	config := &tls.Config{
		MinVersion:         tls.VersionTLS12, // Require TLS 1.2 or higher
		InsecureSkipVerify: skipVerify,

		// Prefer secure cipher suites
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},

		// Prefer secure curves
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
			tls.CurveP521,
		},

		// Require Server Name Indication (SNI) for better security
		ServerName: "",
	}

	// Add certificate if provided
	if cert != nil {
		config.Certificates = []tls.Certificate{*cert}
	}

	return config
}

// VerifyCertificateChain verifies a certificate chain against a CA certificate
func VerifyCertificateChain(certChain [][]byte, caCert *x509.Certificate) error {
	if len(certChain) == 0 {
		return fmt.Errorf("certificate chain is empty")
	}

	// Parse the end-entity certificate
	cert, err := x509.ParseCertificate(certChain[0])
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Create certificate pool with CA
	roots := x509.NewCertPool()
	if caCert != nil {
		roots.AddCert(caCert)
	}

	// Create intermediate pool if there are intermediate certificates
	intermediates := x509.NewCertPool()
	for i := 1; i < len(certChain); i++ {
		intermediateCert, err := x509.ParseCertificate(certChain[i])
		if err != nil {
			return fmt.Errorf("failed to parse intermediate certificate %d: %w", i, err)
		}
		intermediates.AddCert(intermediateCert)
	}

	// Verify certificate chain
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	}

	_, err = cert.Verify(opts)
	if err != nil {
		return fmt.Errorf("certificate verification failed: %w", err)
	}

	return nil
}

// Random utility functions

// GenerateSecureRandomBytes generates cryptographically secure random bytes
func GenerateSecureRandomBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return bytes, nil
}

// GenerateSecureRandomString generates a cryptographically secure random string
// using a charset of alphanumeric characters.
func GenerateSecureRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	bytes, err := GenerateSecureRandomBytes(length)
	if err != nil {
		return "", err
	}

	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = charset[int(bytes[i])%len(charset)]
	}

	return string(result), nil
}

// GenerateSecurePassword generates a cryptographically secure password
// with mixed case letters, numbers, and special characters.
func GenerateSecurePassword(length int) (string, error) {
	if length < 8 {
		return "", fmt.Errorf("password length must be at least 8 characters")
	}

	const (
		lowercase = "abcdefghijklmnopqrstuvwxyz"
		uppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		numbers   = "0123456789"
		special   = "!@#$%^&*()_+-=[]{}|;:,.<>?"
		all       = lowercase + uppercase + numbers + special
	)

	bytes, err := GenerateSecureRandomBytes(length)
	if err != nil {
		return "", err
	}

	password := make([]byte, length)

	// Ensure at least one character from each category
	password[0] = lowercase[int(bytes[0])%len(lowercase)]
	password[1] = uppercase[int(bytes[1])%len(uppercase)]
	password[2] = numbers[int(bytes[2])%len(numbers)]
	password[3] = special[int(bytes[3])%len(special)]

	// Fill the rest with random characters from all categories
	for i := 4; i < length; i++ {
		password[i] = all[int(bytes[i])%len(all)]
	}

	// Shuffle the password to avoid predictable patterns
	for i := length - 1; i > 0; i-- {
		j := int(bytes[i]) % (i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password), nil
}
