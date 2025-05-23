# Revsocks-Modified - Enhanced Embeddable Reverse SOCKS5 Proxy

[![CircleCI](https://circleci.com/gh/dakolli/revsocks-modified.svg?style=svg)](https://circleci.com/gh/dakolli/revsocks-modified)
[![Go Report Card](https://goreportcard.com/badge/github.com/dakolli/revsocks-modified)](https://goreportcard.com/report/github.com/dakolli/revsocks-modified)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/dl/)

> 🚀 **MAJOR REFACTOR**: Version 3.0 introduces a complete architectural overhaul with modern Go practices, enhanced security, and production-ready features.

**Revsocks-Modified** is a production-ready, embeddable reverse SOCKS5 proxy with advanced tunneling capabilities. This enhanced version provides a complete refactor with modern Go architecture, making it suitable for both standalone use and embedding in larger applications.

## 🆕 What's New in 3.0

### 🏗️ **Complete Architectural Refactor**
- **Modular Package Structure**: Clean separation of concerns with dedicated packages
- **Modern CLI with Cobra**: Intuitive command structure with shell completion
- **Structured Logging**: Context-aware logging with multiple output formats
- **Configuration Management**: YAML/JSON config files with validation
- **Enhanced Security**: Production-grade TLS with modern cipher suites

### ✨ **New Features**
- **Embeddable Library**: Use revsocks as a Go library in your applications
- **Advanced Crypto Utilities**: Secure password and key generation
- **Comprehensive Validation**: Configuration and input validation
- **Production Monitoring**: Structured logging with metrics support
- **Shell Completion**: Bash, Zsh, Fish, and PowerShell support

## 📋 Features

### 🔐 **Security & Encryption**
- **TLS 1.2+ Encryption** with modern cipher suites
- **Automatic Certificate Generation** with secure defaults
- **Let's Encrypt Integration** for production deployments
- **Cryptographically Secure** password and key generation
- **Certificate Validation** and chain verification

### 🌐 **Transport Protocols**
- **Raw TCP** - Direct TCP connections
- **TLS-Encrypted TCP** - Secure encrypted tunnels
- **WebSocket** - HTTP-compatible transport (with optional TLS)
- **DNS Tunneling** - Covert channels through DNS queries

### 🔄 **Proxy Support**
- **Corporate Proxies** with authentication
- **Basic Authentication** support
- **NTLM Authentication** for Windows domains
- **System Proxy Detection** automatic configuration
- **Configurable Timeouts** and retry logic

### 🔧 **Operational Features**
- **Automatic Reconnection** with configurable backoff
- **Health Monitoring** and connection tracking
- **Graceful Shutdown** with connection draining
- **Resource Management** with proper cleanup
- **Production Logging** with structured output

## 🚀 Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/dakolli/revsocks-modified.git
cd revsocks-modified

# Build the application
make all

# Or install dependencies and build
go mod tidy
go build -o revsocks .
```

### Basic Usage

#### 1. **Server Mode** (Listen for agents)
```bash
# Start server listening for agents
revsocks server --listen :8443 --socks 127.0.0.1:1080 --password mysecret --tls

# With automatic TLS certificate
revsocks server --listen :8443 --socks 127.0.0.1:1080 --password mysecret \
                 --tls --autocert-domain yourdomain.com
```

#### 2. **Client Mode** (Connect to server)
```bash
# Connect client to server
revsocks client --connect server.example.com:8443 --password mysecret --tls

# Connect through corporate proxy
revsocks client --connect server.example.com:8443 --password mysecret --tls \
                 --proxy proxy.corp.com:3128 --proxy-auth "DOMAIN/user:pass"
```

#### 3. **DNS Tunneling**
```bash
# DNS Server (listening for clients)
revsocks dns-server --domain tunnel.example.com --listen :53 \
                    --socks 127.0.0.1:1080 --key <64-char-hex-key>

# DNS Client (connecting through DNS)
revsocks dns-client --domain tunnel.example.com --key <64-char-hex-key>
```

### Utility Commands

```bash
# Generate secure password
revsocks generate-password 32

# Generate DNS encryption key
revsocks generate-key

# Show configuration
revsocks config show

# Create sample config file
revsocks config init ~/.revsocks.yaml
```

## 📖 Architecture Overview

### 🏗️ **Package Structure**
```
revsocks-modified/
├── cmd/                    # Cobra CLI commands
│   ├── root.go            # Root command and global flags
│   ├── server.go          # Server command implementation
│   ├── client.go          # Client command implementation
│   └── dns.go             # DNS tunneling commands
├── pkg/                    # Public library packages
│   ├── config/            # Configuration management
│   ├── crypto/            # TLS and cryptographic utilities
│   ├── server/            # Server implementations
│   ├── client/            # Client implementations
│   ├── proxy/             # Proxy authentication
│   └── dns/               # DNS tunneling
├── internal/              # Private packages
│   ├── logger/           # Structured logging
│   └── utils/            # Common utilities
├── main.go               # Application entry point
├── go.mod                # Go module definition
└── README.md             # Documentation
```

### 🔄 **Data Flow Diagram**
```
┌─────────────┐    TLS/TCP/WS    ┌─────────────┐    SOCKS5    ┌─────────────┐
│   Client    │◄──────────────►│   Server    │◄─────────────►│ SOCKS Client│
│  (Agent)    │     Tunnel      │ (Listener)  │  Local Conn  │   (App)     │
└─────────────┘                 └─────────────┘              └─────────────┘
      │                               │
      │        Corporate Proxy        │
      └──────────────────────────────►│
            (Optional)                │
                                     ┌▼─────────────┐
                                     │ Target Server│
                                     │  (Internet)  │
                                     └──────────────┘
```

## ⚙️ Configuration

### Configuration File Example
```yaml
# Server configuration
server:
  listen: ":8443"
  socks: "127.0.0.1:1080"
  websocket: false

# Client configuration  
client:
  connect: "server.example.com:8443"
  password: "your-secure-password"
  websocket: false
  reconnect:
    max_attempts: 3
    delay_seconds: 30

# TLS configuration
tls:
  enabled: true
  verify: true
  autocert_domain: "yourdomain.com"

# Proxy configuration
proxy:
  address: "proxy.corp.com:3128"
  timeout_ms: 5000
  auth:
    username: "user"
    password: "pass"
    domain: "DOMAIN"

# Logging configuration
logging:
  level: "info"
  format: "text"
  quiet: false
```

### Environment Variables
```bash
# All configuration can be set via environment variables
export REVSOCKS_SERVER_LISTEN=":8443"
export REVSOCKS_CLIENT_PASSWORD="mysecret"
export REVSOCKS_TLS_ENABLED="true"
export REVSOCKS_LOGGING_LEVEL="debug"
```

## 🔧 Advanced Usage

### WebSocket Transport
```bash
# Server with WebSocket
revsocks server --listen :8443 --socks 127.0.0.1:1080 --websocket --tls

# Client connecting via WebSocket  
revsocks client --connect https://server.example.com:8443 --websocket
```

### DNS Tunneling Setup
```bash
# 1. Set up DNS records for your domain
# A record: tunnel.example.com -> your-server-ip

# 2. Start DNS server
revsocks dns-server --domain tunnel.example.com --listen :53 \
                    --socks 127.0.0.1:1080

# 3. Connect DNS client
revsocks dns-client --domain tunnel.example.com
```

### Corporate Proxy Examples
```bash
# Basic authentication
revsocks client --connect server:8443 --proxy proxy.corp.com:3128 \
                 --proxy-auth "username:password"

# NTLM authentication (Windows domain)
revsocks client --connect server:8443 --proxy proxy.corp.com:3128 \
                 --proxy-auth "DOMAIN/username:password"

# System proxy (automatic detection)
revsocks client --connect server:8443 --proxy "."
```

## 🛠️ Development

### Building from Source
```bash
# Install dependencies
make dep

# Build binary
make revsocks

# Build static binary (for containers)
make static

# Cross-compile for multiple platforms
make gox

# Run tests
make test

# Run with coverage
make test-coverage
```

### Development Tools
```bash
# Install development dependencies
make dev-deps

# Format code
make fmt

# Run linter
make lint

# Security scan
make security
```

### Demo Mode
```bash
# Run demo to see new features
make demo

# Generate secure password
make demo-password

# Generate DNS key
make demo-key
```

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/dakolli/revsocks-modified/issues)
- **Documentation**: [Wiki](https://github.com/dakolli/revsocks-modified/wiki)
- **Security**: See [SECURITY.md](SECURITY.md) for reporting security issues