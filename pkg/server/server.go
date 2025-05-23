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

// SharedSOCKS5Manager manages a single shared SOCKS5 port for all agents in the new server
type SharedSOCKS5Manager struct {
	mu         sync.RWMutex
	listener   net.Listener
	port       int
	address    string
	isRunning  bool
	sessions   []*yamux.Session
	agentCount int
	logger     *logger.Logger
	ctx        context.Context
}

// Server represents a reverse SOCKS5 proxy server instance
// It manages agent connections and forwards SOCKS5 traffic through established tunnels.
type Server struct {
	config       *config.Config
	logger       *logger.Logger
	listener     net.Listener
	server       *http.Server // for WebSocket mode
	sessions     []*yamux.Session
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	socksManager *SharedSOCKS5Manager

	// Connection tracking
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

// StartSharedSOCKS5 initializes and starts the shared SOCKS5 listener if not already running
func (s *SharedSOCKS5Manager) StartSharedSOCKS5(address string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		s.logger.Info("SharedSOCKS5 manager already running", logger.Fields{
			"address": s.address,
			"port":    s.port,
		})
		return nil
	}

	s.address = address
	s.port = port
	fullAddress := fmt.Sprintf("%s:%d", address, port)

	// Start the shared SOCKS5 listener
	listener, err := net.Listen("tcp", fullAddress)
	if err != nil {
		s.logger.Error("Failed to start shared SOCKS5 listener", logger.Fields{
			"error":   err.Error(),
			"address": fullAddress,
		})
		return err
	}

	s.listener = listener
	s.isRunning = true
	s.sessions = make([]*yamux.Session, 0)

	s.logger.Info("✅ Started shared SOCKS5 proxy", logger.Fields{
		"address": fullAddress,
	})

	// Start accepting connections in a goroutine
	go s.acceptConnections()
	return nil
}

// AddSession adds a new agent session to the shared pool
func (s *SharedSOCKS5Manager) AddSession(session *yamux.Session, agentAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions = append(s.sessions, session)
	s.agentCount++

	s.logger.Info("📱 Added agent session to shared pool", logger.Fields{
		"agent_addr":   agentAddr,
		"total_agents": s.agentCount,
	})
}

// RemoveSession removes a failed session from the shared pool
func (s *SharedSOCKS5Manager) RemoveSession(session *yamux.Session, agentAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, sess := range s.sessions {
		if sess == session {
			// Remove session from slice
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			s.agentCount--
			s.logger.Info("🗑️ Removed failed session from shared pool", logger.Fields{
				"agent_addr":       agentAddr,
				"remaining_agents": s.agentCount,
			})
			break
		}
	}
}

// GetActiveSession returns the best available session for load balancing
func (s *SharedSOCKS5Manager) GetActiveSession() *yamux.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Simple round-robin: return first available session
	for _, session := range s.sessions {
		if !session.IsClosed() {
			return session
		}
	}

	s.logger.Warn("⚠️ No active sessions available")
	return nil
}

// acceptConnections handles incoming SOCKS5 client connections
func (s *SharedSOCKS5Manager) acceptConnections() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				if s.ctx.Err() != nil {
					return
				}
				s.logger.Error("Failed to accept SOCKS5 connection", logger.Fields{"error": err.Error()})
				continue
			}

			s.logger.Info("🔌 SOCKS5 client connected", logger.Fields{
				"client_addr": conn.RemoteAddr().String(),
			})

			// Handle each client connection in a goroutine
			go s.handleClientConnection(conn)
		}
	}
}

// handleClientConnection manages individual SOCKS5 client connections
func (s *SharedSOCKS5Manager) handleClientConnection(conn net.Conn) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()

	// Get an active agent session
	session := s.GetActiveSession()
	if session == nil {
		s.logger.Error("❌ No active agent sessions - rejecting client", logger.Fields{
			"client_addr": clientAddr,
		})
		return
	}

	// Open a stream to the agent
	stream, err := session.Open()
	if err != nil {
		s.logger.Error("❌ Failed to open yamux stream for client", logger.Fields{
			"client_addr": clientAddr,
			"error":       err.Error(),
		})
		return
	}
	defer stream.Close()

	s.logger.Info("✅ Established tunnel for client", logger.Fields{
		"client_addr": clientAddr,
	})

	// Bidirectional data copying
	done := make(chan bool, 2)

	// Copy client -> agent
	go func() {
		defer func() { done <- true }()
		io.Copy(stream, conn)
		s.logger.Debug("📤 Client->Agent copy completed", logger.Fields{
			"client_addr": clientAddr,
		})
	}()

	// Copy agent -> client
	go func() {
		defer func() { done <- true }()
		io.Copy(conn, stream)
		s.logger.Debug("📥 Agent->Client copy completed", logger.Fields{
			"client_addr": clientAddr,
		})
	}()

	// Wait for either direction to complete
	<-done
	s.logger.Debug("🔚 Connection closed for client", logger.Fields{
		"client_addr": clientAddr,
	})
}

// New creates a new Server instance with the provided configuration
// The server can be started in TCP or WebSocket mode based on configuration.
func New(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	server := &Server{
		config: cfg,
		logger: logger.GetDefault().WithComponent("server"),
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize shared SOCKS5 manager
	server.socksManager = &SharedSOCKS5Manager{
		logger: server.logger.WithComponent("shared-socks5"),
		ctx:    ctx,
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

	// 🚀 CRITICAL FIX: Wait for session to close instead of exiting immediately
	connLogger.Info("Agent session established, waiting for closure", logger.Fields{
		"connection_id": connection.ID,
	})

	// Block until session closes - this prevents immediate cleanup and disconnection
	select {
	case <-session.CloseChan():
		connLogger.Info("Agent session closed naturally", logger.Fields{
			"connection_id": connection.ID,
		})
	case <-s.ctx.Done():
		connLogger.Info("Server shutdown requested, closing session", logger.Fields{
			"connection_id": connection.ID,
		})
	}
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

	// 🚀 CRITICAL FIX: Wait for session to close instead of exiting immediately
	connLogger.Info("WebSocket agent session established, waiting for closure", logger.Fields{
		"connection_id": connection.ID,
	})

	// Block until session closes - this prevents immediate cleanup and disconnection
	select {
	case <-session.CloseChan():
		connLogger.Info("WebSocket agent session closed naturally", logger.Fields{
			"connection_id": connection.ID,
		})
	case <-s.ctx.Done():
		connLogger.Info("Server shutdown requested, closing WebSocket session", logger.Fields{
			"connection_id": connection.ID,
		})
	}

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

// startSOCKSForwarding starts forwarding SOCKS5 traffic for a connection using shared manager
func (s *Server) startSOCKSForwarding(conn *Connection, connLogger *logger.ConnectionLogger) {
	// Parse SOCKS listen address to get the base port and interface
	listenParts := strings.Split(s.config.Server.Socks, ":")
	if len(listenParts) != 2 {
		connLogger.Error("Invalid SOCKS listen address format", logger.Fields{
			"socks_address": s.config.Server.Socks,
		})
		return
	}

	basePort, err := strconv.Atoi(listenParts[1])
	if err != nil {
		connLogger.Error("Invalid SOCKS listen port", logger.Fields{
			"error": err.Error(),
			"port":  listenParts[1],
		})
		return
	}

	// Set ListenPort to base port (always consistent)
	conn.ListenPort = basePort

	// Start shared SOCKS5 manager if not already running
	err = s.socksManager.StartSharedSOCKS5(listenParts[0], basePort)
	if err != nil {
		connLogger.Error("Failed to start shared SOCKS5 manager", logger.Fields{
			"error": err.Error(),
		})
		return
	}

	// Add this session to the shared manager
	s.socksManager.AddSession(conn.Session, conn.RemoteAddr)

	// Monitor session health and clean up when it closes
	go func() {
		<-conn.Session.CloseChan()
		s.socksManager.RemoveSession(conn.Session, conn.RemoteAddr)
		connLogger.Info("Session closed and removed from shared SOCKS5 manager", logger.Fields{
			"connection_id": conn.ID,
		})
	}()
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
