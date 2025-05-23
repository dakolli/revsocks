VERSION=3.0
GIT_COMMIT = $(shell git rev-parse HEAD | cut -c1-7 2>/dev/null || echo "unknown")
BUILD_DATE = $(shell date -u '+%Y-%m-%d_%H:%M:%S')
BUILD_OPTIONS = -ldflags "-X main.AppVersion=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)"
STATIC_OPTIONS = -ldflags "-extldflags='-static' -X main.AppVersion=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)"

# Default target
.PHONY: all
all: revsocks

# Build the main binary
.PHONY: revsocks
revsocks: dep
	go build $(BUILD_OPTIONS) -o revsocks .

# Build static binary
.PHONY: static
static: dep
	CGO_ENABLED=0 go build $(STATIC_OPTIONS) -o revsocks .

# Install dependencies
.PHONY: dep
dep:
	go mod download
	go mod tidy

# Development tools
.PHONY: tools
tools:
	go install github.com/mitchellh/gox@latest
	go install github.com/tcnksm/ghr@latest

# Display version information
.PHONY: ver
ver:
	@echo "Version: $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"

# Create git tag
.PHONY: gittag
gittag:
	git tag v$(VERSION)
	git push --tags origin main

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf dist
	rm -f revsocks

# Create distribution directory
.PHONY: dist
dist:
	mkdir -p dist

# Cross-compile for multiple platforms
.PHONY: gox
gox: tools dist
	CGO_ENABLED=0 gox -osarch="!darwin/386" -ldflags="-s -w -X main.AppVersion=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)" -output="dist/{{.Dir}}_{{.OS}}_{{.Arch}}"

# Cross-compile Windows binaries
.PHONY: goxwin
goxwin: tools dist
	CGO_ENABLED=0 gox -osarch="windows/amd64 windows/386" -ldflags="-s -w -X main.AppVersion=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)" -output="dist/{{.Dir}}_{{.OS}}_{{.Arch}}"

# Docker build using multi-stage build
.PHONY: dokbuild
dokbuild:
	docker run -it --rm -v $(PWD):/app golang:alpine /bin/sh -c 'apk add make file git && git config --global --add safe.directory /app && cd /app && make -B tools && make gox && make goxwin'

# Create draft release
.PHONY: draft
draft: tools
	ghr -draft v$(VERSION) dist/

# Run tests
.PHONY: test
test:
	go test -v -race -covermode atomic -timeout 30s ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	go test -v -race -covermode atomic -coverprofile=coverage.out -timeout 30s ./...
	go tool cover -html=coverage.out -o coverage.html

# Format code
.PHONY: fmt
fmt:
	go fmt ./...

# Vet code
.PHONY: vet
vet:
	go vet ./...

# Install binary to GOPATH/bin
.PHONY: install
install: revsocks
	go install $(BUILD_OPTIONS) .

# Uninstall binary from GOPATH/bin
.PHONY: uninstall
uninstall:
	rm -f $(shell go env GOPATH)/bin/revsocks

# Show help
.PHONY: help
help:
	@echo "Revsocks Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  all         - Build the main binary (default)"
	@echo "  revsocks    - Build the main binary"
	@echo "  static      - Build static binary"
	@echo "  dep         - Install dependencies"
	@echo "  tools       - Install development tools"
	@echo "  test        - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  fmt         - Format code"
	@echo "  vet         - Vet code"
	@echo "  clean       - Clean build artifacts"
	@echo "  gox         - Cross-compile for multiple platforms"
	@echo "  goxwin      - Cross-compile Windows binaries"
	@echo "  dokbuild    - Docker cross-compilation"
	@echo "  install     - Install to GOPATH/bin"
	@echo "  uninstall   - Remove from GOPATH/bin"
	@echo "  ver         - Show version information"
	@echo "  gittag      - Create and push git tag"
	@echo "  draft       - Create draft release"
	@echo "  demo        - Run demonstration of new features"
	@echo "  help        - Show this help"

# Demo targets for showcasing new features
.PHONY: demo-password
demo-password: revsocks
	@echo "Generating secure password..."
	./revsocks generate-password

.PHONY: demo-key
demo-key: revsocks
	@echo "Generating DNS encryption key..."
	./revsocks generate-key

.PHONY: demo
demo: revsocks demo-password demo-key
	@echo ""
	@echo "Revsocks Refactored Demo Complete!"
	@echo "New features demonstrated:"
	@echo "  ✓ Modular architecture with proper package structure"
	@echo "  ✓ Structured logging with context and levels"
	@echo "  ✓ Secure cryptographic utilities"
	@echo "  ✓ Configuration management with validation"
	@echo "  ✓ Production-ready error handling"

# Build the application with production optimizations and security flags
build: clean
	@echo "🔨 Building revsocks with shared SOCKS5 architecture..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-ldflags "-s -w -X main.version=$(shell git describe --tags --always --dirty) -X main.commit=$(shell git rev-parse --short HEAD) -X main.buildTime=$(shell date -u '+%Y-%m-%d_%H:%M:%S')" \
		-trimpath \
		-o revsocks \
		.
	@echo "✅ Build complete: ./revsocks"

# Quick development build
revsocks: 
	@echo "🔨 Quick build..."
	go build -o revsocks .
	@echo "✅ Development build complete"

# Clean build artifacts
clean:
	@rm -f revsocks
	@echo "🧹 Cleaned build artifacts"

# Security-focused build for production deployment  
secure-build: clean
	@echo "🔒 Building with security optimizations..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-ldflags "-s -w -buildid= -X main.version=$(shell git describe --tags --always --dirty)" \
		-trimpath \
		-tags netgo \
		-installsuffix netgo \
		-o revsocks \
		.
	@echo "🛡️ Secure build complete"

.PHONY: build revsocks clean secure-build

