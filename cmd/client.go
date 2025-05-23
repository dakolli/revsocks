// Package cmd provides the client command implementation
// This command starts a reverse SOCKS5 proxy client that connects to servers.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"revsocks-modified/internal/logger"
	"revsocks-modified/pkg/client"
)

// clientCmd represents the client command
var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Start reverse SOCKS5 proxy client",
	Long: `Start a reverse SOCKS5 proxy client that connects to a server.
The client provides local SOCKS5 proxy functionality and forwards traffic through the server.

Examples:
  # Connect to server with TLS encryption
  revsocks client --connect server.example.com:8443 --password mysecret --tls

  # Connect through corporate proxy with NTLM authentication
  revsocks client --connect server.example.com:8443 --password mysecret --tls \
                  --proxy proxy.corp.com:3128 --proxy-auth "DOMAIN/user:pass"

  # Connect via WebSocket transport
  revsocks client --connect https://server.example.com:8443 --websocket --password mysecret

  # Connect with automatic reconnection
  revsocks client --connect server.example.com:8443 --password mysecret --tls \
                  --reconnect-attempts 5 --reconnect-delay 30`,

	RunE: func(cmd *cobra.Command, args []string) error {
		return runClient()
	},
}

// runClient starts the reverse SOCKS5 proxy client
func runClient() error {
	log := getLogger().WithComponent("client")

	log.Info("Starting reverse SOCKS5 proxy client", logger.Fields{
		"server_address": getConfig().Client.Connect,
		"websocket":      getConfig().Client.UseWebSocket,
		"tls_enabled":    getConfig().TLS.Enabled,
		"proxy_address":  getConfig().Proxy.Address,
	})

	// Create client instance
	cli, err := client.New(getConfig())
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Received shutdown signal, stopping client...")
		cancel()
		cli.Shutdown()
	}()

	// Start client connection
	log.Info("Client connecting...")
	if err := cli.Connect(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("client connection error: %w", err)
	}

	log.Info("Client shutdown completed")
	return nil
}

func init() {
	rootCmd.AddCommand(clientCmd)

	// Client-specific flags
	clientCmd.Flags().String("connect", "", "Server address to connect to (required)")
	clientCmd.Flags().String("password", "", "Authentication password")
	clientCmd.Flags().Bool("websocket", false, "Use WebSocket transport")

	// Reconnection flags
	clientCmd.Flags().Int("reconnect-attempts", 3, "Maximum reconnection attempts (0 = infinite)")
	clientCmd.Flags().Int("reconnect-delay", 30, "Delay between reconnection attempts (seconds)")

	// TLS flags
	clientCmd.Flags().Bool("tls", false, "Enable TLS encryption")
	clientCmd.Flags().Bool("tls-verify", true, "Verify TLS certificates")

	// Proxy flags
	clientCmd.Flags().String("proxy", "", "Proxy server address (use '.' for system proxy)")
	clientCmd.Flags().String("proxy-auth", "", "Proxy authentication (username:password or DOMAIN/username:password)")
	clientCmd.Flags().String("proxy-user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36", "Proxy User-Agent string")
	clientCmd.Flags().Int("proxy-timeout", 1000, "Proxy response timeout (milliseconds)")

	// Mark required flags
	clientCmd.MarkFlagRequired("connect")

	// Bind flags to viper
	viper.BindPFlag("client.connect", clientCmd.Flags().Lookup("connect"))
	viper.BindPFlag("client.password", clientCmd.Flags().Lookup("password"))
	viper.BindPFlag("client.websocket", clientCmd.Flags().Lookup("websocket"))
	viper.BindPFlag("client.reconnect.max_attempts", clientCmd.Flags().Lookup("reconnect-attempts"))
	viper.BindPFlag("client.reconnect.delay_seconds", clientCmd.Flags().Lookup("reconnect-delay"))
	viper.BindPFlag("tls.enabled", clientCmd.Flags().Lookup("tls"))
	viper.BindPFlag("tls.verify", clientCmd.Flags().Lookup("tls-verify"))
	viper.BindPFlag("proxy.address", clientCmd.Flags().Lookup("proxy"))
	viper.BindPFlag("proxy.user_agent", clientCmd.Flags().Lookup("proxy-user-agent"))
	viper.BindPFlag("proxy.timeout_ms", clientCmd.Flags().Lookup("proxy-timeout"))

	// Handle proxy authentication parsing
	clientCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		proxyAuth, _ := cmd.Flags().GetString("proxy-auth")
		if proxyAuth != "" {
			if err := parseProxyAuth(proxyAuth); err != nil {
				return fmt.Errorf("invalid proxy authentication format: %w", err)
			}
		}
		return nil
	}
}

// parseProxyAuth parses proxy authentication string and sets appropriate config values
func parseProxyAuth(authStr string) error {
	// Parse DOMAIN/username:password or username:password
	var username, password, domain string

	if strings.Contains(authStr, "/") {
		// DOMAIN/username:password format
		parts := strings.SplitN(authStr, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid domain/username format")
		}
		domain = parts[0]
		authStr = parts[1]
	}

	// Parse username:password
	parts := strings.SplitN(authStr, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid username:password format")
	}
	username = parts[0]
	password = parts[1]

	// Set viper values
	viper.Set("proxy.auth.username", username)
	viper.Set("proxy.auth.password", password)
	if domain != "" {
		viper.Set("proxy.auth.domain", domain)
	}

	return nil
}
