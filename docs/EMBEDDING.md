# Embedding Revsocks in Your Go Applications

This guide shows how to use revsocks as an embeddable library in your Go applications for programmatic control of reverse SOCKS5 proxy functionality.

## 🚀 Quick Start

### Installation

Add revsocks to your Go module:

```bash
go get revsocks-modified
```

### Basic Server Example

```go
package main

import (
    "context"
    "log"
    
    "revsocks-modified/pkg/config"
    "revsocks-modified/pkg/server"
    "revsocks-modified/internal/logger"
)

func main() {
    // Initialize logger
    logConfig := logger.Config{
        Level:     "info",
        Format:    "text",
        Component: "myapp",
    }
    appLogger := logger.New(logConfig)
    logger.SetDefault(appLogger)
    
    // Create configuration
    cfg := config.NewConfig()
    cfg.Server.Listen = ":8443"
    cfg.Server.Socks = "127.0.0.1:1080"
    cfg.Client.Password = "your-secure-password"
    cfg.TLS.Enabled = true
    
    // Create and start server
    srv, err := server.New(cfg)
    if err != nil {
        log.Fatal("Failed to create server:", err)
    }
    
    log.Println("Starting reverse SOCKS5 proxy server...")
    if err := srv.Start(); err != nil {
        log.Fatal("Server error:", err)
    }
}
```

### Basic Client Example

```go
package main

import (
    "log"
    
    "revsocks-modified/pkg/config"
    "revsocks-modified/pkg/client"
    "revsocks-modified/internal/logger"
)

func main() {
    // Initialize logger
    logConfig := logger.Config{
        Level:     "info",
        Format:    "text",
        Component: "myapp",
    }
    appLogger := logger.New(logConfig)
    logger.SetDefault(appLogger)
    
    // Create configuration
    cfg := config.NewConfig()
    cfg.Client.Connect = "server.example.com:8443"
    cfg.Client.Password = "your-secure-password"
    cfg.TLS.Enabled = true
    cfg.Client.Reconnect.MaxAttempts = 5
    cfg.Client.Reconnect.DelaySeconds = 10
    
    // Create and connect client
    cli, err := client.New(cfg)
    if err != nil {
        log.Fatal("Failed to create client:", err)
    }
    
    log.Println("Connecting to reverse SOCKS5 proxy server...")
    if err := cli.Connect(); err != nil {
        log.Fatal("Connection error:", err)
    }
}
```

## 📦 Package Overview

Revsocks provides several packages for different functionality:

### Core Packages (Public API)

- **`pkg/config`** - Configuration management with validation
- **`pkg/server`** - Reverse SOCKS5 proxy server functionality  
- **`pkg/client`** - Reverse SOCKS5 proxy client functionality
- **`pkg/crypto`** - TLS certificate generation and cryptographic utilities

### Internal Packages (Private)

- **`internal/logger`** - Structured logging with multiple formats
- **`cmd`** - CLI commands (Cobra-based)

## 🔧 Advanced Configuration

### Configuration from Files

```go
// Load configuration from YAML file
cfg := config.NewConfig()
viper.SetConfigFile("config.yaml")
viper.ReadInConfig()
viper.Unmarshal(cfg)

// Validate configuration
if err := cfg.Validate(); err != nil {
    log.Fatal("Configuration validation failed:", err)
}
```

### Environment Variables

All configuration options can be set via environment variables with the `REVSOCKS_` prefix:

```bash
export REVSOCKS_SERVER_LISTEN=":8443"
export REVSOCKS_CLIENT_PASSWORD="secure-password"
export REVSOCKS_TLS_ENABLED="true"
export REVSOCKS_LOGGING_LEVEL="debug"
```

### Programmatic Configuration

```go
cfg := config.NewConfig()

// Server configuration
cfg.Server.Listen = ":8443"
cfg.Server.Socks = "127.0.0.1:1080"
cfg.Server.UseWebSocket = false

// Client configuration  
cfg.Client.Connect = "server.example.com:8443"
cfg.Client.Password = "secure-password"
cfg.Client.UseWebSocket = false
cfg.Client.Reconnect.MaxAttempts = 3
cfg.Client.Reconnect.DelaySeconds = 30

// TLS configuration
cfg.TLS.Enabled = true
cfg.TLS.Verify = true
cfg.TLS.CertificateFile = "/path/to/cert"

// Proxy configuration (for client)
cfg.Proxy.Address = "proxy.corp.com:3128"
cfg.Proxy.Auth.Username = "user"
cfg.Proxy.Auth.Password = "pass"
cfg.Proxy.Auth.Domain = "DOMAIN"

// Logging configuration
cfg.Logging.Level = "info"
cfg.Logging.Format = "json"
```

## 🌐 Transport Protocols

### TCP (Default)

```go
cfg := config.NewConfig()
cfg.Server.UseWebSocket = false
cfg.TLS.Enabled = false // Plain TCP

srv, _ := server.New(cfg)
srv.Start()
```

### TLS-Encrypted TCP

```go
cfg := config.NewConfig()
cfg.Server.UseWebSocket = false
cfg.TLS.Enabled = true
cfg.TLS.Verify = true

srv, _ := server.New(cfg)
srv.Start()
```

### WebSocket

```go
cfg := config.NewConfig()
cfg.Server.UseWebSocket = true
cfg.TLS.Enabled = false // HTTP WebSocket

srv, _ := server.New(cfg)
srv.Start()
```

### Secure WebSocket (WSS)

```go
cfg := config.NewConfig()
cfg.Server.UseWebSocket = true
cfg.TLS.Enabled = true

srv, _ := server.New(cfg)
srv.Start()
```

## 🔐 TLS Certificate Management

### Auto-Generated Certificates

```go
// Server automatically generates self-signed certificate
cfg.TLS.Enabled = true
// No certificate file specified - auto-generated

srv, _ := server.New(cfg)
```

### Custom Certificates

```go
// Use custom certificate files
cfg.TLS.Enabled = true
cfg.TLS.CertificateFile = "server" // Will load server.crt and server.key

srv, _ := server.New(cfg)
```

### Programmatic Certificate Generation

```go
import "revsocks-modified/pkg/crypto"

// Generate certificate programmatically
generator := crypto.NewCertificateGenerator()
cert, err := generator.GenerateSelfSignedCertificate(crypto.CertificateOptions{
    Subject: crypto.DefaultServerSubject(),
    Hosts:   []string{"localhost", "127.0.0.1", "example.com"},
    KeySize: 4096,
    ValidDays: 365,
})

// Save to files
crypto.SaveCertificate(cert, "server.crt", "server.key")
```

## 🔄 Proxy Support (Client)

### Corporate Proxies

```go
cfg := config.NewConfig()

// Basic authentication
cfg.Proxy.Address = "proxy.corp.com:3128"
cfg.Proxy.Auth.Username = "user"
cfg.Proxy.Auth.Password = "pass"

// NTLM authentication (Windows domains)
cfg.Proxy.Auth.Domain = "CORPORATE"

cli, _ := client.New(cfg)
cli.Connect()
```

### System Proxy

```go
cfg := config.NewConfig()
cfg.Proxy.Address = "." // Use system proxy settings

cli, _ := client.New(cfg)
cli.Connect()
```

## 📊 Monitoring and Management

### Server Monitoring

```go
srv, _ := server.New(cfg)

// Start server in background
go srv.Start()

// Monitor connections
ticker := time.NewTicker(10 * time.Second)
for range ticker.C {
    connections := srv.GetConnections()
    fmt.Printf("Active connections: %d\n", len(connections))
    
    for _, conn := range connections {
        fmt.Printf("Connection %s from %s on port %d\n", 
            conn.ID, conn.RemoteAddr, conn.ListenPort)
    }
}
```

### Client Status

```go
cli, _ := client.New(cfg)

// Connect in background
go cli.Connect()

// Monitor status
ticker := time.NewTicker(5 * time.Second)
for range ticker.C {
    if cli.IsConnected() {
        fmt.Printf("Connected: %s\n", cli.GetConnectionID())
    } else {
        fmt.Println("Disconnected")
    }
}
```

### Graceful Shutdown

```go
// Setup signal handling
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

go func() {
    <-sigChan
    cancel()
}()

// Start services
srv, _ := server.New(cfg)
go srv.Start()

// Wait for shutdown signal
<-ctx.Done()

// Graceful shutdown
srv.Shutdown()
```

## 🏗️ Integration Patterns

### Microservice Integration

```go
type ProxyService struct {
    server *server.Server
    client *client.Client
    config *config.Config
    logger *logger.Logger
}

func NewProxyService(cfg *config.Config) *ProxyService {
    return &ProxyService{
        config: cfg,
        logger: logger.GetDefault().WithComponent("proxy-service"),
    }
}

func (ps *ProxyService) StartServer() error {
    srv, err := server.New(ps.config)
    if err != nil {
        return err
    }
    ps.server = srv
    
    ps.logger.Info("Starting proxy server")
    return srv.Start()
}

func (ps *ProxyService) StartClient() error {
    cli, err := client.New(ps.config)
    if err != nil {
        return err
    }
    ps.client = cli
    
    ps.logger.Info("Starting proxy client") 
    return cli.Connect()
}

func (ps *ProxyService) Shutdown() error {
    if ps.server != nil {
        ps.server.Shutdown()
    }
    if ps.client != nil {
        ps.client.Shutdown()
    }
    return nil
}
```

### HTTP API Wrapper

```go
type ProxyAPI struct {
    service *ProxyService
}

func (api *ProxyAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
    status := map[string]interface{}{
        "server_active": api.service.server != nil,
        "client_connected": api.service.client != nil && api.service.client.IsConnected(),
    }
    
    if api.service.server != nil {
        status["connections"] = api.service.server.GetConnections()
    }
    
    json.NewEncoder(w).Encode(status)
}

func (api *ProxyAPI) handleStart(w http.ResponseWriter, r *http.Request) {
    mode := r.URL.Query().Get("mode")
    
    switch mode {
    case "server":
        err := api.service.StartServer()
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
    case "client":
        err := api.service.StartClient()
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
    }
    
    w.WriteHeader(200)
}
```

## 🔧 Crypto Utilities

### Password Generation

```go
import "revsocks-modified/pkg/crypto"

// Generate secure password
password, err := crypto.GenerateSecurePassword(32)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Generated password:", password)
```

### Encryption Keys

```go
// Generate DNS encryption key
keyBytes, err := crypto.GenerateSecureRandomBytes(32)
if err != nil {
    log.Fatal(err)
}
key := fmt.Sprintf("%x", keyBytes)
fmt.Println("DNS key:", key)
```

### Random Strings

```go
// Generate random string
randomStr, err := crypto.GenerateSecureRandomString(16)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Random string:", randomStr)
```

## 📝 Logging Integration

### Custom Logger

```go
// Create custom logger
logConfig := logger.Config{
    Level:     "debug",
    Format:    "json",
    Component: "myapp",
    Output:    os.Stdout, // or custom io.Writer
}

appLogger := logger.New(logConfig)
logger.SetDefault(appLogger)
```

### Structured Logging

```go
logger := logger.GetDefault().WithComponent("proxy")

logger.Info("Starting proxy", logger.Fields{
    "mode":    "server",
    "address": ":8443",
    "tls":     true,
})

logger.Error("Connection failed", logger.Fields{
    "error":       err.Error(),
    "remote_addr": "192.168.1.100:12345",
    "attempt":     3,
})
```

### Log Levels

```go
logger.Trace("Detailed debug information")
logger.Debug("Debug information")
logger.Info("General information")
logger.Warn("Warning message")
logger.Error("Error occurred")
logger.Fatal("Fatal error - exits") 
logger.Panic("Panic error - panics")
```

## 🧪 Testing

### Unit Testing

```go
func TestServerCreation(t *testing.T) {
    cfg := config.NewConfig()
    cfg.Server.Listen = ":0" // Random port
    cfg.Server.Socks = "127.0.0.1:0"
    
    srv, err := server.New(cfg)
    assert.NoError(t, err)
    assert.NotNil(t, srv)
}

func TestClientConnection(t *testing.T) {
    cfg := config.NewConfig()
    cfg.Client.Connect = "localhost:8443"
    cfg.Client.Password = "test-password"
    
    cli, err := client.New(cfg)
    assert.NoError(t, err)
    assert.NotNil(t, cli)
}
```

### Integration Testing

```go
func TestReverseProxy(t *testing.T) {
    // Start server
    serverCfg := config.NewConfig()
    serverCfg.Server.Listen = ":8443"
    serverCfg.Server.Socks = "127.0.0.1:1080"
    serverCfg.Client.Password = "test-password"
    
    srv, _ := server.New(serverCfg)
    go srv.Start()
    defer srv.Shutdown()
    
    // Wait for server to start
    time.Sleep(100 * time.Millisecond)
    
    // Start client
    clientCfg := config.NewConfig()
    clientCfg.Client.Connect = "localhost:8443"
    clientCfg.Client.Password = "test-password"
    
    cli, _ := client.New(clientCfg)
    go cli.Connect()
    defer cli.Shutdown()
    
    // Wait for connection
    time.Sleep(500 * time.Millisecond)
    
    // Test SOCKS5 proxy functionality
    assert.True(t, cli.IsConnected())
    
    connections := srv.GetConnections()
    assert.Len(t, connections, 1)
}
```

## 🔍 Troubleshooting

### Enable Debug Logging

```go
cfg.Logging.Level = "debug"
cfg.Logging.Format = "text" // More readable for debugging
```

### Connection Issues

```go
// Check if server is reachable
conn, err := net.Dial("tcp", cfg.Client.Connect)
if err != nil {
    log.Printf("Cannot reach server: %v", err)
} else {
    conn.Close()
    log.Println("Server is reachable")
}
```

### TLS Certificate Issues

```go
// Disable TLS verification for testing
cfg.TLS.Verify = false

// Or check certificate manually
tlsConfig := crypto.GetSecureTLSConfig(nil, true)
conn, err := tls.Dial("tcp", address, tlsConfig)
```

### Proxy Issues

```go
// Test system proxy detection
systemProxy, err := http.ProxyFromEnvironment(&http.Request{
    URL: &url.URL{Scheme: "https", Host: cfg.Client.Connect},
})
if err != nil {
    log.Printf("System proxy error: %v", err)
} else if systemProxy != nil {
    log.Printf("System proxy: %s", systemProxy.String())
}
```

## 💡 Best Practices

### 1. Configuration Management

- Always validate configuration with `cfg.Validate()`
- Use environment variables for sensitive data
- Separate configuration files for different environments

### 2. Error Handling

- Check all errors returned by methods
- Use structured logging for better debugging
- Implement proper graceful shutdown

### 3. Security

- Always use TLS in production
- Generate strong passwords with `crypto.GenerateSecurePassword()`
- Validate certificates unless using self-signed for testing
- Use proper authentication tokens

### 4. Monitoring

- Monitor connection counts and status
- Log important events with structured fields
- Implement health checks for your application

### 5. Performance

- Use connection pooling for high-throughput scenarios
- Monitor memory usage and connection counts
- Consider using WebSocket for firewall traversal

## 📚 API Reference

For detailed API documentation, see the individual package documentation:

- [pkg/config](../pkg/config/) - Configuration management
- [pkg/server](../pkg/server/) - Server functionality
- [pkg/client](../pkg/client/) - Client functionality  
- [pkg/crypto](../pkg/crypto/) - Cryptographic utilities
- [internal/logger](../internal/logger/) - Logging utilities

## 🤝 Contributing

When extending revsocks for embedding:

1. Keep the public API in `pkg/` packages stable
2. Use structured logging with appropriate levels
3. Implement proper error handling and validation
4. Add comprehensive tests for new functionality
5. Document new features and breaking changes

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/dakolli/revsocks-modified/issues)
- **Documentation**: [Project Wiki](https://github.com/dakolli/revsocks-modified/wiki)
- **Examples**: See `examples/` directory in the repository 