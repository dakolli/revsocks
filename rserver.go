package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"bufio"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/yamux"

	"context"
	"net/http"
	"sync"

	"nhooyr.io/websocket"

	"path/filepath"

	"golang.org/x/crypto/acme/autocert"
)

var proxytout = time.Millisecond * 1000 //timeout for wait magicbytes

// SharedSOCKS5Manager manages a single shared SOCKS5 port for all agents
type SharedSOCKS5Manager struct {
	mu         sync.RWMutex
	listener   net.Listener
	port       int
	address    string
	isRunning  bool
	sessions   []*yamux.Session
	agentCount int
}

// Global shared SOCKS5 manager instance
var sharedSOCKSManager *SharedSOCKS5Manager

// StartSharedSOCKS5 initializes and starts the shared SOCKS5 listener if not already running
func (s *SharedSOCKS5Manager) StartSharedSOCKS5(address string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		log.Printf("[SHARED-SOCKS5] Manager already running on %s:%d", s.address, s.port)
		return nil
	}

	s.address = address
	s.port = port
	fullAddress := fmt.Sprintf("%s:%d", address, port)

	// Start the shared SOCKS5 listener
	listener, err := net.Listen("tcp", fullAddress)
	if err != nil {
		log.Printf("[SHARED-SOCKS5] ERROR: Failed to start shared SOCKS5 listener on %s: %v", fullAddress, err)
		return err
	}

	s.listener = listener
	s.isRunning = true
	s.sessions = make([]*yamux.Session, 0)

	log.Printf("[SHARED-SOCKS5] ✅ Started shared SOCKS5 proxy on %s", fullAddress)

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

	log.Printf("[SHARED-SOCKS5] 📱 Added agent session from %s (total agents: %d)", agentAddr, s.agentCount)
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
			log.Printf("[SHARED-SOCKS5] 🗑️  Removed failed session from %s (remaining agents: %d)", agentAddr, s.agentCount)
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

	log.Printf("[SHARED-SOCKS5] ⚠️  No active sessions available")
	return nil
}

// acceptConnections handles incoming SOCKS5 client connections
func (s *SharedSOCKS5Manager) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("[SHARED-SOCKS5] ERROR: Failed to accept connection: %v", err)
			return
		}

		log.Printf("[SHARED-SOCKS5] 🔌 SOCKS5 client connected from %s", conn.RemoteAddr())

		// Handle each client connection in a goroutine
		go s.handleClientConnection(conn)
	}
}

// handleClientConnection manages individual SOCKS5 client connections
func (s *SharedSOCKS5Manager) handleClientConnection(conn net.Conn) {
	defer conn.Close()

	// Get an active agent session
	session := s.GetActiveSession()
	if session == nil {
		log.Printf("[SHARED-SOCKS5] ❌ No active agent sessions - rejecting client %s", conn.RemoteAddr())
		return
	}

	// Open a stream to the agent
	stream, err := session.Open()
	if err != nil {
		log.Printf("[SHARED-SOCKS5] ❌ Failed to open yamux stream for client %s: %v", conn.RemoteAddr(), err)
		return
	}
	defer stream.Close()

	log.Printf("[SHARED-SOCKS5] ✅ Established tunnel for client %s", conn.RemoteAddr())

	// Bidirectional data copying
	done := make(chan bool, 2)

	// Copy client -> agent
	go func() {
		defer func() { done <- true }()
		io.Copy(stream, conn)
		log.Printf("[SHARED-SOCKS5] 📤 Client->Agent copy completed for %s", conn.RemoteAddr())
	}()

	// Copy agent -> client
	go func() {
		defer func() { done <- true }()
		io.Copy(conn, stream)
		log.Printf("[SHARED-SOCKS5] 📥 Agent->Client copy completed for %s", conn.RemoteAddr())
	}()

	// Wait for either direction to complete
	<-done
	log.Printf("[SHARED-SOCKS5] 🔚 Connection closed for client %s", conn.RemoteAddr())
}

type agentHandler struct {
	mu        sync.Mutex
	listenstr string // listen string for clients
	basePort  int    // base port for SOCKS5 (no longer incrementing)
	timeout   time.Duration
	sessions  []*yamux.Session // all sessions
}

func (h *agentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var session *yamux.Session
	var erry error

	agentstr := r.RemoteAddr
	log.Printf("[%s] Got HTTP request (%s):  %s", agentstr, r.Method, r.URL.String())

	if r.Header.Get("Upgrade") != "websocket" {
		w.Header().Set("Location", "https://www.microsoft.com/")
		w.WriteHeader(http.StatusFound)
		return
	}

	if r.Header.Get("Accept-Language") != agentpassword {
		w.Header().Set("Location", "https://www.microsoft.com/")
		w.WriteHeader(http.StatusFound)
		return
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("[%s] Error upgrading to socket (%s):  %v", agentstr, r.RemoteAddr, err)
		http.Error(w, "Bad request - Go away!", 500)
		return
	}
	defer c.CloseNow()

	if h.timeout > 0 {
		_, cancel := context.WithTimeout(r.Context(), time.Second*60)
		defer cancel()
	}

	nc_over_ws := websocket.NetConn(context.Background(), c, websocket.MessageBinary)

	//Add connection to yamux
	session, erry = yamux.Client(nc_over_ws, nil)
	if erry != nil {
		log.Printf("[%s] Error creating client in yamux for (%s): %v", agentstr, r.RemoteAddr, erry)
		http.Error(w, "Bad request - Go away!", 500)
		return
	}

	h.sessions = append(h.sessions, session)

	// Initialize shared SOCKS5 manager if not already done
	if sharedSOCKSManager == nil {
		sharedSOCKSManager = &SharedSOCKS5Manager{}
	}

	// Start or reuse shared SOCKS5 listener on the base port
	err = sharedSOCKSManager.StartSharedSOCKS5(h.listenstr, h.basePort)
	if err != nil {
		log.Printf("[%s] Failed to start shared SOCKS5 manager: %v", agentstr, err)
		return
	}

	// Add this session to the shared manager
	sharedSOCKSManager.AddSession(session, agentstr)

	// Wait for session to close, then clean up
	<-session.CloseChan()
	sharedSOCKSManager.RemoveSession(session, agentstr)

	c.Close(websocket.StatusNormalClosure, "")
}

func listenForWebsocketAgents(tlslisten bool, address string, clients string, certificate string, autocertdomain string) error {
	var cer tls.Certificate
	var err error
	log.Printf("Will start listening for clients on %s", clients)
	var listenstr = strings.Split(clients, ":")
	portnum, errc := strconv.Atoi(listenstr[1])
	if errc != nil {
		log.Printf("Error converting listen str %s: %v", clients, errc)
	}

	aHandler := &agentHandler{
		basePort:  portnum,
		listenstr: listenstr[0],
	}
	server := &http.Server{
		Addr:    address, // e.g. ":8443"
		Handler: aHandler,
	}
	if tlslisten {
		if autocertdomain != "" {
			log.Printf("Getting TLS certificate for %s", autocertdomain)
			dirname, err := os.UserHomeDir()
			if err != nil {
				log.Printf("Error getting TLS certificate for %s: %v", autocertdomain, err)
			}
			cachepath := filepath.Join(dirname, ".revsocks-autocert")
			m := &autocert.Manager{
				Cache:  autocert.DirCache(cachepath),
				Prompt: autocert.AcceptTOS,
				// Email:      "example@example.org",
				HostPolicy: autocert.HostWhitelist(autocertdomain),
			}
			server.TLSConfig = m.TLSConfig()
		} else {
			if certificate == "" {
				cer, err = getRandomTLS(2048)
				log.Println("No TLS certificate. Generated random one.")
			} else {
				cer, err = tls.LoadX509KeyPair(certificate+".crt", certificate+".key")
			}
			if err != nil {
				log.Printf("Error creating/loading certificate file %s: %v", certificate, err)
				return err
			}
			// config := &tls.Config{Certificates: []tls.Certificate{cer}}
			server.TLSConfig = &tls.Config{
				Certificates: []tls.Certificate{cer},
			}
		}
	}

	log.Printf("Listening for websocket agents on %s (TLS: %t)", address, tlslisten)
	if tlslisten {
		err = server.ListenAndServeTLS("", "")
	} else {
		err = server.ListenAndServe()
	}

	return nil
}

// listen for agents
func listenForAgents(tlslisten bool, address string, clients string, certificate string, autocertdomain string) error {
	var err, erry error
	var cer tls.Certificate
	var session *yamux.Session
	var sessions []*yamux.Session
	var ln net.Listener

	log.Printf("Will start listening for clients on %s and agents on %s (TLS: %t)", clients, address, tlslisten)
	if tlslisten {
		if autocertdomain != "" {
			log.Printf("Getting TLS certificate for %s", autocertdomain)
			dirname, err := os.UserHomeDir()
			if err != nil {
				log.Printf("Error getting TLS certificate for %s: %v", autocertdomain, err)
			}
			cachepath := filepath.Join(dirname, ".revsocks-autocert")
			m := &autocert.Manager{
				Cache:  autocert.DirCache(cachepath),
				Prompt: autocert.AcceptTOS,
				// Email:      "example@example.org",
				HostPolicy: autocert.HostWhitelist(autocertdomain),
			}
			ln, err = tls.Listen("tcp", address, m.TLSConfig())
		} else {
			if certificate == "" {
				cer, err = getRandomTLS(2048)
				log.Println("No TLS certificate. Generated random one.")
			} else {
				cer, err = tls.LoadX509KeyPair(certificate+".crt", certificate+".key")
			}
			if err != nil {
				log.Println(err)
				return err
			}
			config := &tls.Config{Certificates: []tls.Certificate{cer}}
			ln, err = tls.Listen("tcp", address, config)
		}
	} else {
		ln, err = net.Listen("tcp", address)
	}
	if err != nil {
		log.Printf("Error listening on %s: %v", address, err)
		return err
	}
	var listenstr = strings.Split(clients, ":")
	portnum, errc := strconv.Atoi(listenstr[1])
	if errc != nil {
		log.Printf("Error converting listen str %s: %v", clients, errc)
	}

	// Initialize shared SOCKS5 manager
	if sharedSOCKSManager == nil {
		sharedSOCKSManager = &SharedSOCKS5Manager{}
	}

	// Start shared SOCKS5 listener on the base port
	err = sharedSOCKSManager.StartSharedSOCKS5(listenstr[0], portnum)
	if err != nil {
		log.Printf("Failed to start shared SOCKS5 manager: %v", err)
		return err
	}

	for {
		conn, err := ln.Accept()
		conn.RemoteAddr()
		agentstr := conn.RemoteAddr().String()
		log.Printf("[%s] Got a connection from %v: ", agentstr, conn.RemoteAddr())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Errors accepting!")
		}

		reader := bufio.NewReader(conn)

		//read only 64 bytes with timeout=1-3 sec. So we haven't delay with browsers
		conn.SetReadDeadline(time.Now().Add(proxytout))
		statusb := make([]byte, 64)
		_, _ = io.ReadFull(reader, statusb)

		//Alternatively  - read all bytes with timeout=1-3 sec. So we have delay with browsers, but get all GET request
		//conn.SetReadDeadline(time.Now().Add(proxytout))
		//statusb,_ := ioutil.ReadAll(magicBuf)

		//log.Printf("magic bytes: %v",statusb[:6])
		//if hex.EncodeToString(statusb) != magicbytes {
		if string(statusb)[:len(agentpassword)] != agentpassword {
			//do HTTP checks
			log.Printf("Received request: %v", string(statusb[:64]))
			status := string(statusb)
			if strings.Contains(status, " HTTP/1.1") {
				httpresonse := "HTTP/1.1 301 Moved Permanently" +
					"\r\nContent-Type: text/html; charset=UTF-8" +
					"\r\nLocation: https://www.microsoft.com/" +
					"\r\nServer: Apache" +
					"\r\nContent-Length: 0" +
					"\r\nConnection: close" +
					"\r\n\r\n"

				conn.Write([]byte(httpresonse))
				conn.Close()
			} else {
				conn.Close()
			}

		} else {
			//magic bytes received.
			//disable socket read timeouts
			log.Printf("[%s] Got Client from %s", agentstr, conn.RemoteAddr())
			conn.SetReadDeadline(time.Now().Add(100 * time.Hour))
			//Add connection to yamux
			session, erry = yamux.Client(conn, nil)
			if erry != nil {
				log.Printf("[%s] Error creating client in yamux for %s: %v", agentstr, conn.RemoteAddr(), erry)
				continue
			}
			sessions = append(sessions, session)

			// Add session to shared manager instead of starting individual listeners
			sharedSOCKSManager.AddSession(session, agentstr)

			// Monitor session in a goroutine and clean up when it closes
			go func(sess *yamux.Session, addr string) {
				<-sess.CloseChan()
				sharedSOCKSManager.RemoveSession(sess, addr)
				log.Printf("[%s] Session closed and removed from shared manager", addr)
			}(session, agentstr)
		}
	}
	return nil
}

// DEPRECATED: This function is no longer needed with the shared SOCKS5 manager
// Keeping for compatibility but it should not be called in the new architecture
func listenForClients(agentstr string, listen string, port int, session *yamux.Session) error {
	log.Printf("[DEPRECATED] listenForClients called - this should use SharedSOCKS5Manager instead")
	return fmt.Errorf("deprecated function - use SharedSOCKS5Manager")
}
