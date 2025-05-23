// Package client provides embeddable reverse SOCKS5 proxy client functionality
// It can be used as a library in other Go applications for programmatic control.
package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/armon/go-socks5"
	"github.com/hashicorp/yamux"
	"nhooyr.io/websocket"

	"revsocks-modified/internal/logger"
	"revsocks-modified/pkg/config"
	"revsocks-modified/pkg/crypto"

	ntlmssp "github.com/kost/go-ntlmssp"
)

// Client represents a reverse SOCKS5 proxy client instance
// It connects to servers and provides local SOCKS5 proxy functionality.
type Client struct {
	config      *config.Config
	logger      *logger.Logger
	socksServer *socks5.Server
	ctx         context.Context
	cancel      context.CancelFunc
	session     *yamux.Session
	reconnectCh chan struct{}

	// Connection state
	connected    bool
	connectionID string
}

// New creates a new Client instance with the provided configuration
// The client can connect via TCP, TLS, or WebSocket transports.
func New(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Create SOCKS5 server
	socksConfig := &socks5.Config{}

	socksServer, err := socks5.New(socksConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create SOCKS5 server: %w", err)
	}

	client := &Client{
		config:      cfg,
		logger:      logger.GetDefault().WithComponent("client"),
		socksServer: socksServer,
		ctx:         ctx,
		cancel:      cancel,
		reconnectCh: make(chan struct{}, 1),
	}

	return client, nil
}

// Connect establishes connection to the reverse proxy server
// It supports automatic reconnection based on configuration.
func (c *Client) Connect() error {
	c.logger.Info("Starting reverse SOCKS5 proxy client", logger.Fields{
		"server_address": c.config.Client.Connect,
		"websocket":      c.config.Client.UseWebSocket,
		"tls_enabled":    c.config.TLS.Enabled,
		"proxy_address":  c.config.Proxy.Address,
	})

	// Start connection with reconnection logic
	return c.connectWithReconnect()
}

// connectWithReconnect handles connection with automatic reconnection
func (c *Client) connectWithReconnect() error {
	attempt := 0
	maxAttempts := c.config.Client.Reconnect.MaxAttempts

	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}

		attempt++

		c.logger.Info("Attempting to connect to server", logger.Fields{
			"attempt":      attempt,
			"max_attempts": maxAttempts,
		})

		err := c.establishConnection()
		if err == nil {
			c.logger.Info("Successfully connected to server")
			attempt = 0 // Reset attempt counter on successful connection

			// Handle connection until it's lost
			c.handleConnection()

			// If we reach here, connection was lost
			if c.ctx.Err() != nil {
				return c.ctx.Err()
			}
		} else {
			c.logger.Error("Failed to connect to server", logger.Fields{
				"error":   err.Error(),
				"attempt": attempt,
			})
		}

		// Check if we should retry
		if maxAttempts > 0 && attempt >= maxAttempts {
			return fmt.Errorf("maximum connection attempts (%d) exceeded", maxAttempts)
		}

		// Wait before reconnecting
		delayDuration := c.config.GetReconnectDelay()
		c.logger.Info("Waiting before reconnection", logger.Fields{
			"delay_seconds": delayDuration.Seconds(),
		})

		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(delayDuration):
			// Continue to next attempt
		}
	}
}

// establishConnection creates the actual connection to the server
func (c *Client) establishConnection() error {
	if c.config.Client.UseWebSocket {
		return c.connectWebSocket()
	}
	return c.connectTCP()
}

// connectTCP establishes a TCP connection to the server
func (c *Client) connectTCP() error {
	var conn net.Conn
	var err error

	// Connect through proxy if configured
	if c.config.Proxy.Address != "" {
		conn, err = c.connectViaProxy()
		if err != nil {
			return fmt.Errorf("failed to connect via proxy: %w", err)
		}
	} else {
		// Direct connection
		if c.config.TLS.Enabled {
			tlsConfig := crypto.GetSecureTLSConfig(nil, !c.config.TLS.Verify)
			conn, err = tls.Dial("tcp", c.config.Client.Connect, tlsConfig)
		} else {
			conn, err = net.Dial("tcp", c.config.Client.Connect)
		}

		if err != nil {
			return fmt.Errorf("failed to connect to server: %w", err)
		}
	}

	// Send authentication token
	if c.config.Client.Password != "" {
		conn.Write([]byte(c.config.Client.Password))
	}

	// Create yamux session
	c.session, err = yamux.Server(conn, nil)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create yamux session: %w", err)
	}

	c.connected = true
	c.connectionID = generateConnectionID()

	c.logger.Info("TCP connection established", logger.Fields{
		"connection_id": c.connectionID,
	})

	return nil
}

// connectWebSocket establishes a WebSocket connection to the server
func (c *Client) connectWebSocket() error {
	// Create HTTP client for WebSocket connection
	httpClient := &http.Client{}

	// Configure TLS if enabled
	if c.config.TLS.Enabled {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: crypto.GetSecureTLSConfig(nil, !c.config.TLS.Verify),
		}
	}

	// Configure proxy if specified
	if c.config.Proxy.Address != "" {
		err := c.configureHTTPClientProxy(httpClient)
		if err != nil {
			return fmt.Errorf("failed to configure proxy: %w", err)
		}
	}

	// Connect to WebSocket endpoint
	headers := http.Header{
		"User-Agent":      []string{c.config.Proxy.UserAgent},
		"Accept-Language": []string{c.config.Client.Password}, // Legacy compatibility
	}

	wsConn, _, err := websocket.Dial(c.ctx, c.config.Client.Connect, &websocket.DialOptions{
		HTTPClient:   httpClient,
		HTTPHeader:   headers,
		Subprotocols: []string{"chat"}, // Legacy compatibility
	})
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	// Convert to net.Conn
	netConn := websocket.NetConn(c.ctx, wsConn, websocket.MessageBinary)

	// Create yamux session
	c.session, err = yamux.Server(netConn, nil)
	if err != nil {
		wsConn.CloseNow()
		return fmt.Errorf("failed to create yamux session: %w", err)
	}

	c.connected = true
	c.connectionID = generateConnectionID()

	c.logger.Info("WebSocket connection established", logger.Fields{
		"connection_id": c.connectionID,
	})

	return nil
}

// connectViaProxy establishes connection through a corporate proxy
func (c *Client) connectViaProxy() (net.Conn, error) {
	proxyAddr := c.config.Proxy.Address
	targetAddr := c.config.Client.Connect

	// Handle system proxy
	if proxyAddr == "." {
		systemProxy, err := getSystemProxy("POST", "https://"+targetAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to get system proxy: %w", err)
		}
		if systemProxy != nil {
			proxyAddr = systemProxy.Host
		}
	}

	c.logger.Debug("Connecting via proxy", logger.Fields{
		"proxy_address":  proxyAddr,
		"target_address": targetAddr,
	})

	// Connect to proxy
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy: %w", err)
	}

	// Send CONNECT request
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n",
		targetAddr, targetAddr, c.config.Proxy.UserAgent)

	conn.Write([]byte(connectReq))

	// Read response with timeout
	conn.SetReadDeadline(time.Now().Add(c.config.GetProxyTimeout()))
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read proxy response: %w", err)
	}

	// Remove read deadline
	conn.SetReadDeadline(time.Time{})

	// Handle authentication if required
	if resp.StatusCode == 407 {
		conn.Close()
		return c.authenticateProxy(proxyAddr, targetAddr)
	}

	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("proxy returned status: %s", resp.Status)
	}

	// Apply TLS if enabled
	if c.config.TLS.Enabled {
		tlsConfig := crypto.GetSecureTLSConfig(nil, !c.config.TLS.Verify)
		tlsConn := tls.Client(conn, tlsConfig)
		err = tlsConn.Handshake()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("TLS handshake failed: %w", err)
		}
		return tlsConn, nil
	}

	return conn, nil
}

// authenticateProxy handles proxy authentication (NTLM/Basic)
func (c *Client) authenticateProxy(proxyAddr, targetAddr string) (net.Conn, error) {
	if c.config.Proxy.Auth.Username == "" || c.config.Proxy.Auth.Password == "" {
		return nil, fmt.Errorf("proxy authentication required but no credentials provided")
	}

	c.logger.Debug("Authenticating with proxy", logger.Fields{
		"proxy_address": proxyAddr,
		"username":      c.config.Proxy.Auth.Username,
		"domain":        c.config.Proxy.Auth.Domain,
	})

	// Try NTLM authentication first
	if c.config.Proxy.Auth.Domain != "" {
		return c.authenticateNTLM(proxyAddr, targetAddr)
	}

	// Fall back to Basic authentication
	return c.authenticateBasic(proxyAddr, targetAddr)
}

// authenticateNTLM handles NTLM proxy authentication
func (c *Client) authenticateNTLM(proxyAddr, targetAddr string) (net.Conn, error) {
	// Generate negotiate message
	negMsg, err := ntlmssp.NewNegotiateMessage(c.config.Proxy.Auth.Domain, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create NTLM negotiate message: %w", err)
	}

	// Connect to proxy
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy: %w", err)
	}

	// Send CONNECT with negotiate message
	negAuth := fmt.Sprintf("NTLM %s", base64.StdEncoding.EncodeToString(negMsg))
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nProxy-Authorization: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n",
		targetAddr, targetAddr, c.config.Proxy.UserAgent, negAuth)

	conn.Write([]byte(connectReq))

	// Read challenge response
	conn.SetReadDeadline(time.Now().Add(c.config.GetProxyTimeout()))
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read proxy challenge: %w", err)
	}

	if resp.StatusCode != 407 {
		conn.Close()
		return nil, fmt.Errorf("expected 407 status for NTLM challenge, got %d", resp.StatusCode)
	}

	// Parse challenge
	authHeader := resp.Header.Get("Proxy-Authenticate")
	if !strings.HasPrefix(authHeader, "NTLM ") {
		conn.Close()
		return nil, fmt.Errorf("invalid NTLM challenge header")
	}

	challengeData, err := base64.StdEncoding.DecodeString(authHeader[5:])
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to decode NTLM challenge: %w", err)
	}

	// Process challenge
	authMsg, err := ntlmssp.ProcessChallenge(challengeData, c.config.Proxy.Auth.Username, c.config.Proxy.Auth.Password)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to process NTLM challenge: %w", err)
	}

	// Send authentication response
	authResponse := fmt.Sprintf("NTLM %s", base64.StdEncoding.EncodeToString(authMsg))
	connectReq = fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nProxy-Authorization: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n",
		targetAddr, targetAddr, c.config.Proxy.UserAgent, authResponse)

	conn.Write([]byte(connectReq))

	// Read final response
	resp, err = http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read proxy auth response: %w", err)
	}

	conn.SetReadDeadline(time.Time{})

	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("proxy authentication failed: %s", resp.Status)
	}

	return conn, nil
}

// authenticateBasic handles Basic proxy authentication
func (c *Client) authenticateBasic(proxyAddr, targetAddr string) (net.Conn, error) {
	// Create Basic auth header
	authString := fmt.Sprintf("%s:%s", c.config.Proxy.Auth.Username, c.config.Proxy.Auth.Password)
	authHeader := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(authString)))

	// Connect to proxy
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy: %w", err)
	}

	// Send CONNECT with Basic auth
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nProxy-Authorization: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n",
		targetAddr, targetAddr, c.config.Proxy.UserAgent, authHeader)

	conn.Write([]byte(connectReq))

	// Read response
	conn.SetReadDeadline(time.Now().Add(c.config.GetProxyTimeout()))
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read proxy response: %w", err)
	}

	conn.SetReadDeadline(time.Time{})

	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("proxy authentication failed: %s", resp.Status)
	}

	return conn, nil
}

// configureHTTPClientProxy configures proxy settings for HTTP client (WebSocket)
func (c *Client) configureHTTPClientProxy(client *http.Client) error {
	proxyAddr := c.config.Proxy.Address

	// Handle system proxy
	if proxyAddr == "." {
		if client.Transport == nil {
			client.Transport = &http.Transport{}
		}
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.Proxy = http.ProxyFromEnvironment
		}
		return nil
	}

	// Parse proxy URL
	if !strings.HasPrefix(proxyAddr, "http://") && !strings.HasPrefix(proxyAddr, "https://") {
		proxyAddr = "http://" + proxyAddr
	}

	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	// Add authentication if configured
	if c.config.Proxy.Auth.Username != "" && c.config.Proxy.Auth.Password != "" {
		proxyURL.User = url.UserPassword(c.config.Proxy.Auth.Username, c.config.Proxy.Auth.Password)
	}

	// Configure transport
	if client.Transport == nil {
		client.Transport = &http.Transport{}
	}

	if transport, ok := client.Transport.(*http.Transport); ok {
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return nil
}

// handleConnection manages the established connection and SOCKS5 forwarding
func (c *Client) handleConnection() {
	defer func() {
		c.connected = false
		if c.session != nil {
			c.session.Close()
			c.session = nil
		}
	}()

	// Handle yamux streams
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		stream, err := c.session.Accept()
		if err != nil {
			c.logger.Error("Failed to accept yamux stream", logger.Fields{
				"error":         err.Error(),
				"connection_id": c.connectionID,
			})
			return
		}

		c.logger.Debug("Accepted yamux stream", logger.Fields{
			"connection_id": c.connectionID,
		})

		// Handle stream with SOCKS5 server
		go func(stream net.Conn) {
			defer stream.Close()

			err := c.socksServer.ServeConn(stream)
			if err != nil {
				c.logger.Debug("SOCKS5 stream closed", logger.Fields{
					"error": err.Error(),
				})
			}
		}(stream)
	}
}

// getSystemProxy retrieves system proxy configuration
func getSystemProxy(method, urlStr string) (*url.URL, error) {
	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return nil, err
	}

	return http.ProxyFromEnvironment(req)
}

// IsConnected returns true if the client is connected to the server
func (c *Client) IsConnected() bool {
	return c.connected
}

// GetConnectionID returns the current connection ID
func (c *Client) GetConnectionID() string {
	return c.connectionID
}

// Shutdown gracefully shuts down the client
func (c *Client) Shutdown() error {
	c.logger.Info("Shutting down client")

	// Cancel context to stop goroutines
	c.cancel()

	// Close session if exists
	if c.session != nil {
		c.session.Close()
	}

	c.logger.Info("Client shutdown complete")
	return nil
}

// generateConnectionID creates a unique connection identifier
func generateConnectionID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}
