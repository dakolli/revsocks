// Package config provides comprehensive configuration management for revsocks
// with validation, environment variable support, and structured configuration.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// Config represents the complete application configuration
// It provides structured access to all configuration options with proper validation.
type Config struct {
	// Server configuration for listening mode
	Server ServerConfig `mapstructure:"server" json:"server" yaml:"server"`

	// Client configuration for connecting mode
	Client ClientConfig `mapstructure:"client" json:"client" yaml:"client"`

	// DNS tunneling configuration
	DNS DNSConfig `mapstructure:"dns" json:"dns" yaml:"dns"`

	// TLS configuration
	TLS TLSConfig `mapstructure:"tls" json:"tls" yaml:"tls"`

	// Proxy configuration
	Proxy ProxyConfig `mapstructure:"proxy" json:"proxy" yaml:"proxy"`

	// Logging configuration
	Logging LoggingConfig `mapstructure:"logging" json:"logging" yaml:"logging"`

	// General application settings
	General GeneralConfig `mapstructure:"general" json:"general" yaml:"general"`
}

// ServerConfig contains all server-side configuration options
type ServerConfig struct {
	// Listen address for incoming agent connections (e.g., ":8443")
	Listen string `mapstructure:"listen" json:"listen" yaml:"listen" validate:"required"`

	// SOCKS5 bind address for client connections (default: "127.0.0.1:1080")
	Socks string `mapstructure:"socks" json:"socks" yaml:"socks"`

	// Use WebSocket transport instead of raw TCP
	UseWebSocket bool `mapstructure:"websocket" json:"websocket" yaml:"websocket"`
}

// ClientConfig contains all client-side configuration options
type ClientConfig struct {
	// Remote server address to connect to (e.g., "server.example.com:8443")
	Connect string `mapstructure:"connect" json:"connect" yaml:"connect" validate:"required"`

	// Authentication password for server connection
	Password string `mapstructure:"password" json:"password" yaml:"password"`

	// Use WebSocket transport instead of raw TCP
	UseWebSocket bool `mapstructure:"websocket" json:"websocket" yaml:"websocket"`

	// Reconnection settings
	Reconnect ReconnectConfig `mapstructure:"reconnect" json:"reconnect" yaml:"reconnect"`
}

// ReconnectConfig manages client reconnection behavior
type ReconnectConfig struct {
	// Maximum number of reconnection attempts (0 = infinite)
	MaxAttempts int `mapstructure:"max_attempts" json:"max_attempts" yaml:"max_attempts"`

	// Delay between reconnection attempts in seconds
	DelaySeconds int `mapstructure:"delay_seconds" json:"delay_seconds" yaml:"delay_seconds"`
}

// DNSConfig contains DNS tunneling configuration
type DNSConfig struct {
	// Domain name for DNS tunneling (e.g., "tunnel.example.com")
	Domain string `mapstructure:"domain" json:"domain" yaml:"domain"`

	// DNS server listen address (server mode only, e.g., ":53")
	Listen string `mapstructure:"listen" json:"listen" yaml:"listen"`

	// Delay between DNS requests in milliseconds
	DelayMs int `mapstructure:"delay_ms" json:"delay_ms" yaml:"delay_ms"`

	// Encryption key for DNS payloads (64-character hex string)
	EncryptionKey string `mapstructure:"encryption_key" json:"encryption_key" yaml:"encryption_key"`
}

// TLSConfig contains TLS/SSL configuration options
type TLSConfig struct {
	// Enable TLS encryption for connections
	Enabled bool `mapstructure:"enabled" json:"enabled" yaml:"enabled"`

	// Verify TLS certificates (disable for self-signed)
	Verify bool `mapstructure:"verify" json:"verify" yaml:"verify"`

	// Path to TLS certificate file (without extension, .crt and .key will be appended)
	CertificateFile string `mapstructure:"certificate_file" json:"certificate_file" yaml:"certificate_file"`

	// Domain for automatic Let's Encrypt certificate generation
	AutoCertDomain string `mapstructure:"autocert_domain" json:"autocert_domain" yaml:"autocert_domain"`
}

// ProxyConfig contains HTTP proxy configuration
type ProxyConfig struct {
	// Proxy server address (e.g., "proxy.corp.com:3128", use "." for system proxy)
	Address string `mapstructure:"address" json:"address" yaml:"address"`

	// Authentication credentials
	Auth ProxyAuthConfig `mapstructure:"auth" json:"auth" yaml:"auth"`

	// Proxy response timeout in milliseconds
	TimeoutMs int `mapstructure:"timeout_ms" json:"timeout_ms" yaml:"timeout_ms"`

	// User-Agent string for proxy requests
	UserAgent string `mapstructure:"user_agent" json:"user_agent" yaml:"user_agent"`
}

// ProxyAuthConfig contains proxy authentication settings
type ProxyAuthConfig struct {
	// Username for proxy authentication
	Username string `mapstructure:"username" json:"username" yaml:"username"`

	// Password for proxy authentication
	Password string `mapstructure:"password" json:"password" yaml:"password"`

	// Domain for NTLM authentication (optional)
	Domain string `mapstructure:"domain" json:"domain" yaml:"domain"`
}

// LoggingConfig contains logging configuration options
type LoggingConfig struct {
	// Log level: trace, debug, info, warn, error, fatal, panic
	Level string `mapstructure:"level" json:"level" yaml:"level"`

	// Log format: text, json
	Format string `mapstructure:"format" json:"format" yaml:"format"`

	// Disable all output (quiet mode)
	Quiet bool `mapstructure:"quiet" json:"quiet" yaml:"quiet"`

	// Enable debug output
	Debug bool `mapstructure:"debug" json:"debug" yaml:"debug"`
}

// GeneralConfig contains general application settings
type GeneralConfig struct {
	// Application version information
	Version  string `mapstructure:"version" json:"version" yaml:"version"`
	CommitID string `mapstructure:"commit_id" json:"commit_id" yaml:"commit_id"`
}

// NewConfig creates a new configuration instance with sensible defaults
// This function initializes all configuration sections with production-ready defaults.
func NewConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Socks: "127.0.0.1:1080",
		},
		Client: ClientConfig{
			Reconnect: ReconnectConfig{
				MaxAttempts:  3,
				DelaySeconds: 30,
			},
		},
		DNS: DNSConfig{
			DelayMs: 200,
		},
		TLS: TLSConfig{
			Verify: true,
		},
		Proxy: ProxyConfig{
			TimeoutMs: 1000,
			UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// Validate performs comprehensive validation of the configuration
// It checks for required fields, validates network addresses, and ensures
// configuration consistency across different operation modes.
func (c *Config) Validate() error {
	// Validate server configuration if in server mode
	if c.Server.Listen != "" {
		if err := c.validateServerConfig(); err != nil {
			return fmt.Errorf("server configuration validation failed: %w", err)
		}
	}

	// Validate client configuration if in client mode
	if c.Client.Connect != "" {
		if err := c.validateClientConfig(); err != nil {
			return fmt.Errorf("client configuration validation failed: %w", err)
		}
	}

	// Validate DNS configuration if DNS tunneling is enabled
	if c.DNS.Domain != "" {
		if err := c.validateDNSConfig(); err != nil {
			return fmt.Errorf("DNS configuration validation failed: %w", err)
		}
	}

	// Validate TLS configuration
	if err := c.validateTLSConfig(); err != nil {
		return fmt.Errorf("TLS configuration validation failed: %w", err)
	}

	// Validate proxy configuration
	if err := c.validateProxyConfig(); err != nil {
		return fmt.Errorf("proxy configuration validation failed: %w", err)
	}

	// Validate logging configuration
	if err := c.validateLoggingConfig(); err != nil {
		return fmt.Errorf("logging configuration validation failed: %w", err)
	}

	return nil
}

// validateServerConfig validates server-specific configuration
func (c *Config) validateServerConfig() error {
	// Validate listen address format
	if _, err := net.ResolveTCPAddr("tcp", c.Server.Listen); err != nil {
		return fmt.Errorf("invalid listen address '%s': %w", c.Server.Listen, err)
	}

	// Validate SOCKS bind address format
	if _, err := net.ResolveTCPAddr("tcp", c.Server.Socks); err != nil {
		return fmt.Errorf("invalid SOCKS address '%s': %w", c.Server.Socks, err)
	}

	return nil
}

// validateClientConfig validates client-specific configuration
func (c *Config) validateClientConfig() error {
	// Handle WebSocket URLs
	if c.Client.UseWebSocket {
		if _, err := url.Parse(c.Client.Connect); err != nil {
			return fmt.Errorf("invalid WebSocket URL '%s': %w", c.Client.Connect, err)
		}
	} else {
		// Validate TCP address format
		if _, err := net.ResolveTCPAddr("tcp", c.Client.Connect); err != nil {
			return fmt.Errorf("invalid connect address '%s': %w", c.Client.Connect, err)
		}
	}

	// Validate reconnection settings
	if c.Client.Reconnect.MaxAttempts < 0 {
		return fmt.Errorf("max reconnection attempts cannot be negative")
	}

	if c.Client.Reconnect.DelaySeconds < 0 {
		return fmt.Errorf("reconnection delay cannot be negative")
	}

	return nil
}

// validateDNSConfig validates DNS tunneling configuration
func (c *Config) validateDNSConfig() error {
	// Validate domain name format
	if c.DNS.Domain == "" {
		return fmt.Errorf("DNS domain cannot be empty")
	}

	// Validate DNS listen address if in server mode
	if c.DNS.Listen != "" {
		if _, err := net.ResolveTCPAddr("tcp", c.DNS.Listen); err != nil {
			return fmt.Errorf("invalid DNS listen address '%s': %w", c.DNS.Listen, err)
		}
	}

	// Validate encryption key format (must be 64-character hex string)
	if c.DNS.EncryptionKey != "" && len(c.DNS.EncryptionKey) != 64 {
		return fmt.Errorf("DNS encryption key must be exactly 64 characters (hex)")
	}

	// Validate delay settings
	if c.DNS.DelayMs < 0 {
		return fmt.Errorf("DNS delay cannot be negative")
	}

	return nil
}

// validateTLSConfig validates TLS configuration
func (c *Config) validateTLSConfig() error {
	// If autocert is enabled, validate domain
	if c.TLS.AutoCertDomain != "" {
		if !isValidDomain(c.TLS.AutoCertDomain) {
			return fmt.Errorf("invalid autocert domain '%s'", c.TLS.AutoCertDomain)
		}
	}

	return nil
}

// validateProxyConfig validates proxy configuration
func (c *Config) validateProxyConfig() error {
	// Validate proxy address if specified
	if c.Proxy.Address != "" && c.Proxy.Address != "." {
		if c.Client.UseWebSocket {
			// For WebSocket, proxy should be HTTP URL
			if _, err := url.Parse(c.Proxy.Address); err != nil {
				return fmt.Errorf("invalid proxy URL '%s': %w", c.Proxy.Address, err)
			}
		} else {
			// For TCP, proxy should be host:port
			if _, err := net.ResolveTCPAddr("tcp", c.Proxy.Address); err != nil {
				return fmt.Errorf("invalid proxy address '%s': %w", c.Proxy.Address, err)
			}
		}
	}

	// Validate timeout
	if c.Proxy.TimeoutMs < 0 {
		return fmt.Errorf("proxy timeout cannot be negative")
	}

	return nil
}

// validateLoggingConfig validates logging configuration
func (c *Config) validateLoggingConfig() error {
	// Validate log level
	validLevels := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
	levelValid := false
	for _, level := range validLevels {
		if c.Logging.Level == level {
			levelValid = true
			break
		}
	}
	if !levelValid {
		return fmt.Errorf("invalid log level '%s', must be one of: %s",
			c.Logging.Level, strings.Join(validLevels, ", "))
	}

	// Validate log format
	if c.Logging.Format != "text" && c.Logging.Format != "json" {
		return fmt.Errorf("invalid log format '%s', must be 'text' or 'json'", c.Logging.Format)
	}

	return nil
}

// GetProxyTimeout returns the proxy timeout as a time.Duration
func (c *Config) GetProxyTimeout() time.Duration {
	return time.Duration(c.Proxy.TimeoutMs) * time.Millisecond
}

// GetDNSDelay returns the DNS delay as a time.Duration
func (c *Config) GetDNSDelay() time.Duration {
	return time.Duration(c.DNS.DelayMs) * time.Millisecond
}

// GetReconnectDelay returns the reconnection delay as a time.Duration
func (c *Config) GetReconnectDelay() time.Duration {
	return time.Duration(c.Client.Reconnect.DelaySeconds) * time.Second
}

// IsServerMode returns true if configuration is for server mode
func (c *Config) IsServerMode() bool {
	return c.Server.Listen != ""
}

// IsClientMode returns true if configuration is for client mode
func (c *Config) IsClientMode() bool {
	return c.Client.Connect != ""
}

// IsDNSMode returns true if configuration is for DNS tunneling mode
func (c *Config) IsDNSMode() bool {
	return c.DNS.Domain != ""
}

// isValidDomain performs basic domain name validation
func isValidDomain(domain string) bool {
	// Basic domain validation - could be enhanced with more sophisticated checks
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}

	// Check for valid characters and structure
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 || len(part) > 63 {
			return false
		}
	}

	return true
}

// ParseProxyAuth parses proxy authentication string in format "domain/user:pass" or "user:pass"
// Returns domain, username, password, and any parsing error.
func ParseProxyAuth(authString string) (domain, username, password string, err error) {
	if authString == "" {
		return "", "", "", nil
	}

	// Handle domain/user:pass format
	if strings.Contains(authString, "/") {
		parts := strings.SplitN(authString, "/", 2)
		if len(parts) != 2 {
			return "", "", "", fmt.Errorf("invalid proxy auth format with domain")
		}

		domain = parts[0]
		userPass := parts[1]

		userPassParts := strings.SplitN(userPass, ":", 2)
		if len(userPassParts) != 2 {
			return "", "", "", fmt.Errorf("invalid user:pass format")
		}

		username = userPassParts[0]
		password = userPassParts[1]
	} else {
		// Handle user:pass format
		parts := strings.SplitN(authString, ":", 2)
		if len(parts) != 2 {
			return "", "", "", fmt.Errorf("invalid user:pass format")
		}

		username = parts[0]
		password = parts[1]
	}

	return domain, username, password, nil
}
