// Package server provides embeddable reverse SOCKS5 proxy server functionality
// It can be used as a library in other Go applications for programmatic control.
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"nhooyr.io/websocket"

	"revsocks-modified/internal/logger"
	"revsocks-modified/pkg/config"
	"revsocks-modified/pkg/crypto"
)

// Server represents a reverse SOCKS5 proxy server instance
// It manages agent connections and forwards SOCKS5 traffic through established tunnels.
type Server struct {
	config   *config.Config
	logger   *logger.Logger
	listener net.Listener
	server   *http.Server // for WebSocket mode
	sessions []*yamux.Session
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc

	// Connection tracking
	nextPort    int
	activeConns sync.Map // map[string]*Connection for tracking active connections
}

// Connection represents an active agent connection
type Connection struct {
	ID          string
	RemoteAddr  string
	Session     *yamux.Session
	ListenPort  int
	ConnectedAt time.Time
	BytesIn     int64
	BytesOut    int64
}

// New creates a new Server instance with the provided configuration
// The server can be started in TCP or WebSocket mode based on configuration.
func New(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Parse listen port for client connections
	listenParts := strings.Split(cfg.Server.Socks, ":")
	if len(listenParts) != 2 {
		return nil, fmt.Errorf("invalid SOCKS listen address format: %s", cfg.Server.Socks)
	}

	basePort, err := strconv.Atoi(listenParts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid SOCKS listen port: %w", err)
	}

	server := &Server{
		config:   cfg,
		logger:   logger.GetDefault().WithComponent("server"),
		ctx:      ctx,
		cancel:   cancel,
		nextPort: basePort,
	}

	return server, nil
}

// Start begins listening for agent connections and serving SOCKS5 clients
// It supports both TCP and WebSocket transports based on configuration.
func (s *Server) Start() error {
	s.logger.Info("Starting reverse SOCKS5 proxy server", logger.Fields{
		"listen_address": s.config.Server.Listen,
		"socks_address":  s.config.Server.Socks,
		"websocket":      s.config.Server.UseWebSocket,
		"tls_enabled":    s.config.TLS.Enabled,
	})

	if s.config.Server.UseWebSocket {
		return s.startWebSocketServer()
	}
	return s.startTCPServer()
}

// startTCPServer starts the TCP-based server for agent connections
func (s *Server) startTCPServer() error {
	var err error

	// Create TLS configuration if enabled
	if s.config.TLS.Enabled {
		tlsConfig, err := s.createTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to create TLS configuration: %w", err)
		}

		s.listener, err = tls.Listen("tcp", s.config.Server.Listen, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to start TLS listener on %s: %w", s.config.Server.Listen, err)
		}

		s.logger.Info("TLS listener started", logger.Fields{
			"address": s.config.Server.Listen,
		})
	} else {
		s.listener, err = net.Listen("tcp", s.config.Server.Listen)
		if err != nil {
			return fmt.Errorf("failed to start TCP listener on %s: %w", s.config.Server.Listen, err)
		}

		s.logger.Info("TCP listener started", logger.Fields{
			"address": s.config.Server.Listen,
		})
	}

	// Accept connections in background
	go s.acceptConnections()

	// Wait for context cancellation
	<-s.ctx.Done()
	return s.Shutdown()
}

// startWebSocketServer starts the WebSocket-based server for agent connections
func (s *Server) startWebSocketServer() error {
	// Create HTTP server with WebSocket handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	s.server = &http.Server{
		Addr:    s.config.Server.Listen,
		Handler: mux,
	}

	// Configure TLS if enabled
	if s.config.TLS.Enabled {
		tlsConfig, err := s.createTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to create TLS configuration: %w", err)
		}
		s.server.TLSConfig = tlsConfig
	}

	s.logger.Info("WebSocket server starting", logger.Fields{
		"address":     s.config.Server.Listen,
		"tls_enabled": s.config.TLS.Enabled,
	})

	// Start server in background
	go func() {
		var err error
		if s.config.TLS.Enabled {
			err = s.server.ListenAndServeTLS("", "")
		} else {
			err = s.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			s.logger.Error("WebSocket server error", logger.Fields{"error": err.Error()})
		}
	}()

	// Wait for context cancellation
	<-s.ctx.Done()
	return s.Shutdown()
}

// acceptConnections handles incoming TCP connections from agents
func (s *Server) acceptConnections() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				if s.ctx.Err() != nil {
					return // Server shutting down
				}
				s.logger.Error("Failed to accept connection", logger.Fields{"error": err.Error()})
				continue
			}

			// Handle connection in background
			go s.handleTCPConnection(conn)
		}
	}
}

// handleTCPConnection processes an incoming TCP connection from an agent
func (s *Server) handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	connLogger := logger.NewConnectionLogger(s.logger, remoteAddr, generateConnectionID())

	connLogger.Info("Agent connection received")

	// Read authentication token with timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	authBuffer := make([]byte, 256)
	n, err := conn.Read(authBuffer)
	if err != nil {
		connLogger.Error("Failed to read authentication", logger.Fields{"error": err.Error()})
		return
	}

	// Validate authentication
	authToken := string(authBuffer[:n])
	if !s.validateAuthentication(authToken) {
		connLogger.Warn("Invalid authentication token")
		s.sendHTTPRedirect(conn)
		return
	}

	// Remove read deadline
	conn.SetReadDeadline(time.Time{})

	// Create yamux session
	session, err := yamux.Client(conn, nil)
	if err != nil {
		connLogger.Error("Failed to create yamux session", logger.Fields{"error": err.Error()})
		return
	}

	// Register connection
	connection := &Connection{
		ID:          generateConnectionID(),
		RemoteAddr:  remoteAddr,
		Session:     session,
		ConnectedAt: time.Now(),
	}

	s.registerConnection(connection)
	defer s.unregisterConnection(connection.ID)

	// Start SOCKS5 forwarding
	s.startSOCKSForwarding(connection, connLogger)
}

// handleWebSocket processes WebSocket connections from agents
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	remoteAddr := r.RemoteAddr
	connLogger := logger.NewConnectionLogger(s.logger, remoteAddr, generateConnectionID())

	connLogger.Info("WebSocket connection received", logger.Fields{
		"method": r.Method,
		"url":    r.URL.String(),
	})

	// Check for WebSocket upgrade
	if r.Header.Get("Upgrade") != "websocket" {
		s.sendHTTPRedirect(w)
		return
	}

	// Validate authentication via header
	authToken := r.Header.Get("Accept-Language") // Legacy compatibility
	if !s.validateAuthentication(authToken) {
		connLogger.Warn("Invalid authentication token")
		s.sendHTTPRedirect(w)
		return
	}

	// Accept WebSocket connection
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"chat"}, // Legacy compatibility
	})
	if err != nil {
		connLogger.Error("Failed to accept WebSocket", logger.Fields{"error": err.Error()})
		return
	}
	defer conn.CloseNow()

	// Convert to net.Conn
	netConn := websocket.NetConn(context.Background(), conn, websocket.MessageBinary)

	// Create yamux session
	session, err := yamux.Client(netConn, nil)
	if err != nil {
		connLogger.Error("Failed to create yamux session", logger.Fields{"error": err.Error()})
		return
	}

	// Register connection
	connection := &Connection{
		ID:          generateConnectionID(),
		RemoteAddr:  remoteAddr,
		Session:     session,
		ConnectedAt: time.Now(),
	}

	s.registerConnection(connection)
	defer s.unregisterConnection(connection.ID)

	// Start SOCKS5 forwarding
	s.startSOCKSForwarding(connection, connLogger)

	// Close WebSocket gracefully
	conn.Close(websocket.StatusNormalClosure, "Connection closed")
}

// validateAuthentication checks if the provided token is valid
func (s *Server) validateAuthentication(token string) bool {
	expectedPassword := s.config.Client.Password
	if expectedPassword == "" {
		return true // No authentication required
	}

	return strings.TrimSpace(token) == expectedPassword
}

// sendHTTPRedirect sends an HTTP redirect response (for steganography)
func (s *Server) sendHTTPRedirect(w interface{}) {
	redirectResponse := "HTTP/1.1 301 Moved Permanently\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"Location: https://www.microsoft.com/\r\n" +
		"Server: Apache\r\n" +
		"Content-Length: 0\r\n" +
		"Connection: close\r\n\r\n"

	switch conn := w.(type) {
	case net.Conn:
		conn.Write([]byte(redirectResponse))
	case http.ResponseWriter:
		conn.Header().Set("Location", "https://www.microsoft.com/")
		conn.WriteHeader(http.StatusMovedPermanently)
	}
}

// startSOCKSForwarding starts forwarding SOCKS5 traffic for a connection
func (s *Server) startSOCKSForwarding(conn *Connection, connLogger *logger.ConnectionLogger) {
	// Get next available port
	s.mu.Lock()
	port := s.nextPort
	s.nextPort++
	s.mu.Unlock()

	// Parse SOCKS listen address
	listenParts := strings.Split(s.config.Server.Socks, ":")
	listenAddr := fmt.Sprintf("%s:%d", listenParts[0], port)

	conn.ListenPort = port

	connLogger.Info("Starting SOCKS5 listener", logger.Fields{
		"socks_address": listenAddr,
	})

	// Start listening for SOCKS5 clients
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		connLogger.Error("Failed to start SOCKS5 listener", logger.Fields{
			"error":   err.Error(),
			"address": listenAddr,
		})
		return
	}
	defer listener.Close()

	// Accept SOCKS5 clients
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			client, err := listener.Accept()
			if err != nil {
				if s.ctx.Err() != nil {
					return
				}
				connLogger.Error("Failed to accept SOCKS5 client", logger.Fields{"error": err.Error()})
				continue
			}

			// Handle client in background
			go s.handleSOCKSClient(conn, client, connLogger)
		}
	}
}

// handleSOCKSClient forwards a SOCKS5 client through the agent tunnel
func (s *Server) handleSOCKSClient(conn *Connection, client net.Conn, connLogger *logger.ConnectionLogger) {
	defer client.Close()

	clientAddr := client.RemoteAddr().String()
	connLogger.Info("SOCKS5 client connected", logger.Fields{
		"client_address": clientAddr,
	})

	// Open stream through yamux
	stream, err := conn.Session.Open()
	if err != nil {
		connLogger.Error("Failed to open yamux stream", logger.Fields{
			"error":          err.Error(),
			"client_address": clientAddr,
		})
		return
	}
	defer stream.Close()

	// Copy data bidirectionally
	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(client, stream)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(stream, client)
		errChan <- err
	}()

	// Wait for first error (connection closed)
	<-errChan

	connLogger.Debug("SOCKS5 client disconnected", logger.Fields{
		"client_address": clientAddr,
	})
}

// createTLSConfig creates a TLS configuration for the server
func (s *Server) createTLSConfig() (*tls.Config, error) {
	var cert tls.Certificate
	var err error

	if s.config.TLS.CertificateFile != "" {
		// Load certificate from file
		cert, err = crypto.LoadCertificate(
			s.config.TLS.CertificateFile+".crt",
			s.config.TLS.CertificateFile+".key",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}

		s.logger.Info("Loaded TLS certificate from file", logger.Fields{
			"cert_file": s.config.TLS.CertificateFile,
		})
	} else {
		// Generate self-signed certificate
		generator := crypto.NewCertificateGenerator()
		cert, err = generator.GenerateSelfSignedCertificate(crypto.CertificateOptions{
			Subject: crypto.DefaultServerSubject(),
			Hosts:   []string{"localhost", "127.0.0.1"},
			KeySize: 4096,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate certificate: %w", err)
		}

		s.logger.Info("Generated self-signed TLS certificate")
	}

	return crypto.GetSecureTLSConfig(&cert, false), nil
}

// registerConnection adds a connection to the active connections map
func (s *Server) registerConnection(conn *Connection) {
	s.activeConns.Store(conn.ID, conn)
	s.logger.Info("Agent connection registered", logger.Fields{
		"connection_id": conn.ID,
		"remote_addr":   conn.RemoteAddr,
		"listen_port":   conn.ListenPort,
	})
}

// unregisterConnection removes a connection from the active connections map
func (s *Server) unregisterConnection(connID string) {
	s.activeConns.Delete(connID)
	s.logger.Info("Agent connection unregistered", logger.Fields{
		"connection_id": connID,
	})
}

// GetConnections returns a list of active connections
func (s *Server) GetConnections() []*Connection {
	var connections []*Connection
	s.activeConns.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*Connection); ok {
			connections = append(connections, conn)
		}
		return true
	})
	return connections
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	s.logger.Info("Shutting down server")

	// Cancel context to stop goroutines
	s.cancel()

	// Close listener if exists
	if s.listener != nil {
		s.listener.Close()
	}

	// Shutdown HTTP server if exists
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}

	// Close all active sessions
	s.activeConns.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*Connection); ok {
			if conn.Session != nil {
				conn.Session.Close()
			}
		}
		return true
	})

	s.logger.Info("Server shutdown complete")
	return nil
}

// generateConnectionID creates a unique connection identifier
func generateConnectionID() string {
	return fmt.Sprintf("conn_%d", time.Now().UnixNano())
}
