// Package cmd provides the command-line interface for revsocks using Cobra
// It implements a modern CLI with proper command structure, validation, and help system.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"revsocks-modified/internal/logger"
	"revsocks-modified/pkg/config"
	"revsocks-modified/pkg/crypto"
)

var (
	// cfgFile holds the configuration file path
	cfgFile string

	// cfg holds the application configuration
	cfg *config.Config

	// log holds the application logger
	log *logger.Logger

	// Version information (will be set at build time)
	Version   = "dev"
	CommitID  = "unknown"
	BuildTime = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "revsocks",
	Short: "Reverse SOCKS5 proxy with advanced tunneling capabilities",
	Long: `Revsocks is a powerful reverse SOCKS5 proxy tool that enables secure tunneling 
through various protocols including TCP, TLS, WebSocket, and DNS.

Features:
  • Reverse SOCKS5 proxy (client connects back to server)
  • Multiple transport protocols (TCP, TLS, WebSocket, DNS)
  • Corporate proxy support with authentication (Basic, NTLM)
  • Automatic TLS certificate generation or Let's Encrypt integration
  • Robust reconnection handling and error recovery
  • Structured logging with multiple output formats
  • Production-ready security configurations

Examples:
  # Start server listening for agents
  revsocks server --listen :8443 --socks 127.0.0.1:1080 --password mysecret

  # Connect client to server  
  revsocks client --connect server.example.com:8443 --password mysecret

  # Use DNS tunneling
  revsocks dns-server --domain tunnel.example.com --listen :53 --socks 127.0.0.1:1080
  revsocks dns-client --domain tunnel.example.com --key <64-char-hex-key>

  # Connect through corporate proxy
  revsocks client --connect server:8443 --proxy proxy.corp.com:3128 \
                  --proxy-auth "DOMAIN/user:pass" --password mysecret`,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initializeConfig(cmd)
	},

	Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, CommitID, BuildTime),
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file path (default: $HOME/.revsocks.yaml)")

	// Logging configuration
	rootCmd.PersistentFlags().String("log-level", "info",
		"log level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().String("log-format", "text",
		"log format (text, json)")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false,
		"suppress all output except errors")
	rootCmd.PersistentFlags().Bool("debug", false,
		"enable debug logging (shorthand for --log-level=debug)")

	// Bind flags to viper
	viper.BindPFlag("logging.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("logging.format", rootCmd.PersistentFlags().Lookup("log-format"))
	viper.BindPFlag("logging.quiet", rootCmd.PersistentFlags().Lookup("quiet"))
	viper.BindPFlag("logging.debug", rootCmd.PersistentFlags().Lookup("debug"))

	// Set environment variable prefix
	viper.SetEnvPrefix("REVSOCKS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".revsocks" (without extension)
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".revsocks")
	}

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// initializeConfig initializes the application configuration and logger
func initializeConfig(cmd *cobra.Command) error {
	// Create new configuration with defaults
	cfg = config.NewConfig()

	// Set version information
	cfg.General.Version = Version
	cfg.General.CommitID = CommitID

	// Load configuration from viper (files, env vars, flags)
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Handle debug flag override
	if viper.GetBool("logging.debug") {
		cfg.Logging.Level = "debug"
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Initialize logger
	logConfig := logger.Config{
		Level:     cfg.Logging.Level,
		Format:    cfg.Logging.Format,
		Quiet:     cfg.Logging.Quiet,
		Component: "revsocks",
	}

	log = logger.New(logConfig)
	logger.SetDefault(log)

	// Replace standard logger to capture logs from dependencies
	logger.ReplaceStandardLogger(log)

	log.Info("Revsocks starting", logger.Fields{
		"version":    Version,
		"commit_id":  CommitID,
		"build_time": BuildTime,
	})

	return nil
}

// getConfig returns the current configuration instance
func getConfig() *config.Config {
	return cfg
}

// getLogger returns the current logger instance
func getLogger() *logger.Logger {
	return log
}

// generatePasswordCommand generates a secure random password
var generatePasswordCmd = &cobra.Command{
	Use:   "generate-password [length]",
	Short: "Generate a cryptographically secure password",
	Long: `Generate a cryptographically secure password with mixed case letters,
numbers, and special characters. Default length is 32 characters.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		length := 32 // Default length

		if len(args) > 0 {
			// Parse length from argument
			if _, err := fmt.Sscanf(args[0], "%d", &length); err != nil {
				return fmt.Errorf("invalid length: %v", err)
			}
		}

		if length < 8 {
			return fmt.Errorf("password length must be at least 8 characters")
		}

		password, err := crypto.GenerateSecurePassword(length)
		if err != nil {
			return fmt.Errorf("failed to generate password: %w", err)
		}

		fmt.Println(password)
		return nil
	},
}

// generateKeyCommand generates a secure random key for DNS tunneling
var generateKeyCmd = &cobra.Command{
	Use:   "generate-key",
	Short: "Generate a cryptographically secure 64-character hex key for DNS tunneling",
	Long: `Generate a cryptographically secure 64-character hexadecimal key
suitable for DNS tunneling encryption. This key should be shared between
the DNS server and client for secure communication.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Generate 32 random bytes (256 bits) which will be 64 hex characters
		bytes, err := crypto.GenerateSecureRandomBytes(32)
		if err != nil {
			return fmt.Errorf("failed to generate key: %w", err)
		}

		// Convert to hexadecimal string
		key := fmt.Sprintf("%x", bytes)
		fmt.Println(key)
		return nil
	},
}

// versionCmd provides detailed version information
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Long:  "Display detailed version information including build details",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Revsocks %s\n", Version)
		fmt.Printf("Commit: %s\n", CommitID)
		fmt.Printf("Built: %s\n", BuildTime)
		fmt.Printf("Go Version: %s\n", strings.TrimPrefix(fmt.Sprintf("%s", os.Args[0]), "go"))
	},
}

// completionCmd generates shell completion scripts
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for revsocks.

To load completions:

Bash:
  $ source <(revsocks completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ revsocks completion bash > /etc/bash_completion.d/revsocks
  # macOS:
  $ revsocks completion bash > /usr/local/etc/bash_completion.d/revsocks

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ revsocks completion zsh > "${fpath[1]}/_revsocks"
  # You will need to start a new shell for this setup to take effect.

fish:
  $ revsocks completion fish | source
  # To load completions for each session, execute once:
  $ revsocks completion fish > ~/.config/fish/completions/revsocks.fish

PowerShell:
  PS> revsocks completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> revsocks completion powershell > revsocks.ps1
  # and source this file from your PowerShell profile.`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %s", args[0])
		}
	},
}

// configCmd provides configuration management
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
	Long:  "Commands for managing revsocks configuration files and settings",
}

// configShowCmd displays the current configuration
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	Long:  "Display the current configuration including all settings and their sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Display configuration file being used
		if viper.ConfigFileUsed() != "" {
			fmt.Printf("Configuration file: %s\n\n", viper.ConfigFileUsed())
		} else {
			fmt.Println("No configuration file loaded\n")
		}

		// Display all configuration keys and values
		fmt.Println("Current configuration:")
		for _, key := range viper.AllKeys() {
			fmt.Printf("  %s: %v\n", key, viper.Get(key))
		}

		return nil
	},
}

// configInitCmd creates a sample configuration file
var configInitCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Create a sample configuration file",
	Long: `Create a sample configuration file with default values and documentation.
If no path is provided, creates .revsocks.yaml in the current directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := ".revsocks.yaml"
		if len(args) > 0 {
			configPath = args[0]
		}

		// Check if file already exists
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("configuration file already exists: %s", configPath)
		}

		// Create sample configuration content
		sampleConfig := `# Revsocks Configuration File
# This file contains the configuration for revsocks with explanations and examples

# Server configuration (for listening mode)
server:
  listen: ":8443"              # Address to listen for agent connections
  socks: "127.0.0.1:1080"      # SOCKS5 bind address for clients
  websocket: false             # Use WebSocket transport

# Client configuration (for connecting mode)
client:
  connect: ""                  # Server address to connect to
  password: ""                 # Authentication password
  websocket: false             # Use WebSocket transport
  reconnect:
    max_attempts: 3            # Maximum reconnection attempts (0 = infinite)
    delay_seconds: 30          # Delay between reconnection attempts

# DNS tunneling configuration
dns:
  domain: ""                   # Domain for DNS tunneling (e.g., "tunnel.example.com")
  listen: ""                   # DNS server listen address (server mode only)
  delay_ms: 200                # Delay between DNS requests
  encryption_key: ""           # 64-character hex encryption key

# TLS configuration
tls:
  enabled: false               # Enable TLS encryption
  verify: true                 # Verify TLS certificates
  certificate_file: ""         # Path to certificate file (without extension)
  autocert_domain: ""          # Domain for automatic Let's Encrypt certificates

# Proxy configuration
proxy:
  address: ""                  # Proxy server address ("." for system proxy)
  timeout_ms: 1000             # Proxy response timeout
  user_agent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
  auth:
    username: ""               # Proxy username
    password: ""               # Proxy password  
    domain: ""                 # Domain for NTLM authentication

# Logging configuration
logging:
  level: "info"                # Log level (trace, debug, info, warn, error, fatal, panic)
  format: "text"               # Log format (text, json)
  quiet: false                 # Disable all output
  debug: false                 # Enable debug logging
`

		// Create directory if needed
		if dir := filepath.Dir(configPath); dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		}

		// Write configuration file
		if err := os.WriteFile(configPath, []byte(sampleConfig), 0644); err != nil {
			return fmt.Errorf("failed to write configuration file: %w", err)
		}

		fmt.Printf("Sample configuration file created: %s\n", configPath)
		fmt.Println("Edit this file to configure revsocks for your environment.")

		return nil
	},
}

func init() {
	// Add utility commands
	rootCmd.AddCommand(generatePasswordCmd)
	rootCmd.AddCommand(generateKeyCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(completionCmd)

	// Add configuration commands
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	rootCmd.AddCommand(configCmd)
}
