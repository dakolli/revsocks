// Package main demonstrates how to use revsocks as an embeddable library
// This example shows programmatic usage without the CLI interface.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"revsocks-modified/internal/logger"
	"revsocks-modified/pkg/client"
	"revsocks-modified/pkg/config"
	"revsocks-modified/pkg/crypto"
	"revsocks-modified/pkg/server"
)

func main() {
	fmt.Println("🚀 Revsocks Embeddable Library Demo")
	fmt.Println("This demonstrates using revsocks programmatically in your Go applications")
	fmt.Println()

	// Initialize logger
	logConfig := logger.Config{
		Level:     "info",
		Format:    "text",
		Component: "demo",
	}
	appLogger := logger.New(logConfig)
	logger.SetDefault(appLogger)

	// Create wait group for coordinating server and client
	var wg sync.WaitGroup

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		appLogger.Info("Received shutdown signal, stopping...")
		cancel()
	}()

	// Example 1: Start a server programmatically
	fmt.Println("📡 Starting Embedded Server Example...")
	wg.Add(1)
	go func() {
		defer wg.Done()
		runServerExample(ctx)
	}()

	// Wait a moment for server to start
	time.Sleep(2 * time.Second)

	// Example 2: Start a client programmatically
	fmt.Println("🔌 Starting Embedded Client Example...")
	wg.Add(1)
	go func() {
		defer wg.Done()
		runClientExample(ctx)
	}()

	// Example 3: Crypto utilities
	fmt.Println("🔐 Demonstrating Crypto Utilities...")
	runCryptoExample()

	// Wait for shutdown signal
	<-ctx.Done()
	appLogger.Info("Shutting down demo...")

	// Wait for goroutines to finish
	wg.Wait()
	fmt.Println("✅ Demo completed successfully!")
}

// runServerExample demonstrates embedding the server
func runServerExample(ctx context.Context) {
	log := logger.GetDefault().WithComponent("server-example")

	log.Info("Creating embedded server configuration")

	// Create server configuration
	cfg := config.NewConfig()
	cfg.Server.Listen = ":8443"
	cfg.Server.Socks = "127.0.0.1:1080"
	cfg.Client.Password = "demo-password-123"
	cfg.TLS.Enabled = true
	cfg.TLS.Verify = false // Self-signed cert for demo

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Error("Server configuration validation failed", logger.Fields{
			"error": err.Error(),
		})
		return
	}

	// Create and start server
	srv, err := server.New(cfg)
	if err != nil {
		log.Error("Failed to create server", logger.Fields{
			"error": err.Error(),
		})
		return
	}

	log.Info("Server created successfully, starting...")

	// Start server in background
	go func() {
		if err := srv.Start(); err != nil {
			log.Error("Server error", logger.Fields{
				"error": err.Error(),
			})
		}
	}()

	// Monitor connections
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Shutting down server...")
			srv.Shutdown()
			return
		case <-ticker.C:
			connections := srv.GetConnections()
			log.Info("Server status", logger.Fields{
				"active_connections": len(connections),
			})

			for _, conn := range connections {
				log.Debug("Active connection", logger.Fields{
					"id":           conn.ID,
					"remote_addr":  conn.RemoteAddr,
					"listen_port":  conn.ListenPort,
					"connected_at": conn.ConnectedAt.Format(time.RFC3339),
				})
			}
		}
	}
}

// runClientExample demonstrates embedding the client
func runClientExample(ctx context.Context) {
	log := logger.GetDefault().WithComponent("client-example")

	log.Info("Creating embedded client configuration")

	// Create client configuration
	cfg := config.NewConfig()
	cfg.Client.Connect = "localhost:8443"
	cfg.Client.Password = "demo-password-123"
	cfg.TLS.Enabled = true
	cfg.TLS.Verify = false // Skip verification for demo
	cfg.Client.Reconnect.MaxAttempts = 3
	cfg.Client.Reconnect.DelaySeconds = 5

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Error("Client configuration validation failed", logger.Fields{
			"error": err.Error(),
		})
		return
	}

	// Create client
	cli, err := client.New(cfg)
	if err != nil {
		log.Error("Failed to create client", logger.Fields{
			"error": err.Error(),
		})
		return
	}

	log.Info("Client created successfully, connecting...")

	// Connect in background with reconnection
	go func() {
		if err := cli.Connect(); err != nil && ctx.Err() == nil {
			log.Error("Client connection failed", logger.Fields{
				"error": err.Error(),
			})
		}
	}()

	// Monitor connection status
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Shutting down client...")
			cli.Shutdown()
			return
		case <-ticker.C:
			if cli.IsConnected() {
				log.Info("Client status", logger.Fields{
					"connected":     true,
					"connection_id": cli.GetConnectionID(),
				})
			} else {
				log.Info("Client status", logger.Fields{
					"connected": false,
				})
			}
		}
	}
}

// runCryptoExample demonstrates the crypto utilities
func runCryptoExample() {
	log := logger.GetDefault().WithComponent("crypto-example")

	log.Info("Demonstrating crypto utilities")

	// Generate secure password
	password, err := crypto.GenerateSecurePassword(32)
	if err != nil {
		log.Error("Failed to generate password", logger.Fields{
			"error": err.Error(),
		})
		return
	}

	fmt.Printf("🔑 Generated secure password: %s\n", password)

	// Generate encryption key for DNS tunneling
	keyBytes, err := crypto.GenerateSecureRandomBytes(32)
	if err != nil {
		log.Error("Failed to generate key", logger.Fields{
			"error": err.Error(),
		})
		return
	}

	fmt.Printf("🔐 Generated DNS encryption key: %x\n", keyBytes)

	// Generate TLS certificate
	log.Info("Generating self-signed TLS certificate")

	generator := crypto.NewCertificateGenerator()
	cert, err := generator.GenerateSelfSignedCertificate(crypto.CertificateOptions{
		Subject: crypto.DefaultServerSubject(),
		Hosts:   []string{"localhost", "127.0.0.1", "example.com"},
		KeySize: 2048, // Smaller key for demo speed
	})
	if err != nil {
		log.Error("Failed to generate certificate", logger.Fields{
			"error": err.Error(),
		})
		return
	}

	fmt.Printf("📜 Generated TLS certificate with %d bytes\n", len(cert.Certificate[0]))

	// Save certificate to files (optional)
	err = crypto.SaveCertificate(cert, "demo-cert.crt", "demo-cert.key")
	if err != nil {
		log.Warn("Failed to save certificate", logger.Fields{
			"error": err.Error(),
		})
	} else {
		fmt.Println("💾 Certificate saved to demo-cert.crt and demo-cert.key")
	}

	log.Info("Crypto utilities demonstration completed")
}

// Example of how you could integrate revsocks into a larger application:

// MyApplication demonstrates how to embed revsocks in a larger application
type MyApplication struct {
	server *server.Server
	client *client.Client
	logger *logger.Logger
	config *config.Config
}

// NewMyApplication creates a new application instance
func NewMyApplication() *MyApplication {
	return &MyApplication{
		logger: logger.GetDefault().WithComponent("myapp"),
	}
}

// StartReverseProxy starts the reverse proxy functionality
func (app *MyApplication) StartReverseProxy(cfg *config.Config) error {
	app.config = cfg

	if cfg.IsServerMode() {
		srv, err := server.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}
		app.server = srv

		app.logger.Info("Starting reverse proxy server")
		return srv.Start()
	}

	if cfg.IsClientMode() {
		cli, err := client.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		app.client = cli

		app.logger.Info("Starting reverse proxy client")
		return cli.Connect()
	}

	return fmt.Errorf("configuration must specify either server or client mode")
}

// StopReverseProxy stops the reverse proxy functionality
func (app *MyApplication) StopReverseProxy() error {
	if app.server != nil {
		app.logger.Info("Stopping reverse proxy server")
		return app.server.Shutdown()
	}

	if app.client != nil {
		app.logger.Info("Stopping reverse proxy client")
		return app.client.Shutdown()
	}

	return nil
}

// GetProxyStatus returns the current proxy status
func (app *MyApplication) GetProxyStatus() map[string]interface{} {
	status := make(map[string]interface{})

	if app.server != nil {
		connections := app.server.GetConnections()
		status["type"] = "server"
		status["active_connections"] = len(connections)
		status["connections"] = connections
	}

	if app.client != nil {
		status["type"] = "client"
		status["connected"] = app.client.IsConnected()
		status["connection_id"] = app.client.GetConnectionID()
	}

	return status
}
