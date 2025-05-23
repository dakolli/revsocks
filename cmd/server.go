// Package cmd provides the server command implementation
// This command starts a reverse SOCKS5 proxy server that listens for agent connections.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"revsocks-modified/internal/logger"
	"revsocks-modified/pkg/server"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start reverse SOCKS5 proxy server",
	Long: `Start a reverse SOCKS5 proxy server that listens for incoming agent connections.
The server forwards SOCKS5 traffic through established tunnels from connected agents.

Examples:
  # Start server with TLS encryption
  revsocks server --listen :8443 --socks 127.0.0.1:1080 --password mysecret --tls

  # Start server with WebSocket transport
  revsocks server --listen :8443 --socks 127.0.0.1:1080 --websocket --tls

  # Start server with custom certificate
  revsocks server --listen :8443 --socks 127.0.0.1:1080 --cert-file server --tls

  # Start server without TLS (not recommended for production)
  revsocks server --listen :8080 --socks 127.0.0.1:1080 --password mysecret`,

	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer()
	},
}

// runServer starts the reverse SOCKS5 proxy server
func runServer() error {
	log := getLogger().WithComponent("server")

	log.Info("Starting reverse SOCKS5 proxy server", logger.Fields{
		"listen_address": getConfig().Server.Listen,
		"socks_address":  getConfig().Server.Socks,
		"websocket":      getConfig().Server.UseWebSocket,
		"tls_enabled":    getConfig().TLS.Enabled,
	})

	// Create server instance
	srv, err := server.New(getConfig())
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Received shutdown signal, stopping server...")
		cancel()
		srv.Shutdown()
	}()

	// Start server
	log.Info("Server starting up...")
	if err := srv.Start(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("server error: %w", err)
	}

	log.Info("Server shutdown completed")
	return nil
}

func init() {
	rootCmd.AddCommand(serverCmd)

	// Server-specific flags
	serverCmd.Flags().String("listen", ":8443", "Address to listen for agent connections")
	serverCmd.Flags().String("socks", "127.0.0.1:1080", "SOCKS5 bind address for clients")
	serverCmd.Flags().Bool("websocket", false, "Use WebSocket transport")
	serverCmd.Flags().String("password", "", "Authentication password for agents")

	// TLS flags
	serverCmd.Flags().Bool("tls", false, "Enable TLS encryption")
	serverCmd.Flags().Bool("tls-verify", true, "Verify TLS certificates")
	serverCmd.Flags().String("cert-file", "", "TLS certificate file (without extension)")
	serverCmd.Flags().String("autocert-domain", "", "Domain for automatic Let's Encrypt certificates")

	// Bind flags to viper
	viper.BindPFlag("server.listen", serverCmd.Flags().Lookup("listen"))
	viper.BindPFlag("server.socks", serverCmd.Flags().Lookup("socks"))
	viper.BindPFlag("server.websocket", serverCmd.Flags().Lookup("websocket"))
	viper.BindPFlag("client.password", serverCmd.Flags().Lookup("password"))
	viper.BindPFlag("tls.enabled", serverCmd.Flags().Lookup("tls"))
	viper.BindPFlag("tls.verify", serverCmd.Flags().Lookup("tls-verify"))
	viper.BindPFlag("tls.certificate_file", serverCmd.Flags().Lookup("cert-file"))
	viper.BindPFlag("tls.autocert_domain", serverCmd.Flags().Lookup("autocert-domain"))
}
