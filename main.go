// Revsocks - Reverse SOCKS5 proxy with advanced tunneling capabilities
//
// This is the main entry point for the revsocks application.
// It provides secure tunneling through various protocols including TCP, TLS, WebSocket, and DNS.
//
// Architecture:
//   - Server Mode: Listens for incoming agent connections and forwards SOCKS5 traffic
//   - Client Mode: Connects to servers through proxies and handles local SOCKS5 requests
//   - DNS Mode: Uses DNS queries for covert tunneling
//
// Security Features:
//   - TLS encryption with modern cipher suites
//   - Automatic certificate generation or Let's Encrypt integration
//   - Secure random password and key generation
//   - Corporate proxy support with NTLM/Basic authentication
//
// Usage Examples:
//
//	Server: revsocks server --listen :8443 --socks 127.0.0.1:1080 --password mysecret
//	Client: revsocks client --connect server.com:8443 --password mysecret
//	DNS:    revsocks dns-server --domain tunnel.example.com --listen :53
//
// For more information, see: https://github.com/dakolli/revsocks-modified
package main

import (
	"revsocks-modified/cmd"
)

// Build information - these will be set at build time using ldflags
var (
	AppVersion = "dev"
	GitCommit  = "unknown"
	BuildDate  = "unknown"
)

// main is the application entry point that delegates to the Cobra CLI
func main() {
	// Set build information for CLI
	cmd.Version = AppVersion
	cmd.CommitID = GitCommit
	cmd.BuildTime = BuildDate

	// Execute the root command (Cobra CLI)
	cmd.Execute()
}
