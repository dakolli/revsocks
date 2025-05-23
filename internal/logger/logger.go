// Package logger provides structured logging capabilities for revsocks
// with multiple output formats, log levels, and context-aware logging.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// Logger represents the application logger with structured logging capabilities
type Logger struct {
	level     Level
	output    io.Writer
	component string
	formatter Formatter
}

// Fields represents a map of key-value pairs for structured logging
type Fields map[string]interface{}

// Level represents the log level
type Level int

const (
	TraceLevel Level = iota
	DebugLevel
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
	PanicLevel
)

// String returns the string representation of the log level
func (level Level) String() string {
	switch level {
	case TraceLevel:
		return "TRACE"
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	case PanicLevel:
		return "PANIC"
	default:
		return "UNKNOWN"
	}
}

// Formatter interface for log formatting
type Formatter interface {
	Format(level Level, msg string, fields Fields, component string) string
}

// TextFormatter formats logs as human-readable text
type TextFormatter struct{}

// Format implements the Formatter interface for text output
func (f *TextFormatter) Format(level Level, msg string, fields Fields, component string) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelStr := fmt.Sprintf("%-5s", level.String())

	result := fmt.Sprintf("%s [%s]", timestamp, levelStr)

	if component != "" {
		result += fmt.Sprintf(" [%s]", component)
	}

	result += fmt.Sprintf(" %s", msg)

	// Add structured fields
	if len(fields) > 0 {
		var fieldPairs []string
		for k, v := range fields {
			fieldPairs = append(fieldPairs, fmt.Sprintf("%s=%v", k, v))
		}
		result += fmt.Sprintf(" | %s", strings.Join(fieldPairs, " "))
	}

	return result + "\n"
}

// JSONFormatter formats logs as JSON
type JSONFormatter struct{}

// Format implements the Formatter interface for JSON output
func (f *JSONFormatter) Format(level Level, msg string, fields Fields, component string) string {
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")

	jsonFields := make(map[string]interface{})
	jsonFields["timestamp"] = timestamp
	jsonFields["level"] = level.String()
	jsonFields["message"] = msg

	if component != "" {
		jsonFields["component"] = component
	}

	// Add user fields
	for k, v := range fields {
		jsonFields[k] = v
	}

	// Simple JSON formatting (could be enhanced with proper JSON library)
	var parts []string
	for k, v := range jsonFields {
		parts = append(parts, fmt.Sprintf(`"%s":"%v"`, k, v))
	}

	return "{" + strings.Join(parts, ",") + "}\n"
}

// contextKey is the type used for context keys in logging
type contextKey string

const (
	// LoggerContextKey is the context key for storing logger instances
	LoggerContextKey contextKey = "logger"
	// ComponentContextKey is the context key for storing component names
	ComponentContextKey contextKey = "component"
)

// Config represents logger configuration options
type Config struct {
	// Level specifies the minimum log level
	Level string

	// Format specifies the log output format (text, json)
	Format string

	// Output specifies where logs should be written (default: os.Stdout)
	Output io.Writer

	// Quiet disables all logging output
	Quiet bool

	// Component name for this logger instance
	Component string
}

// ParseLevel parses a string log level into a Level
func ParseLevel(levelStr string) (Level, error) {
	switch strings.ToLower(levelStr) {
	case "trace":
		return TraceLevel, nil
	case "debug":
		return DebugLevel, nil
	case "info":
		return InfoLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "error":
		return ErrorLevel, nil
	case "fatal":
		return FatalLevel, nil
	case "panic":
		return PanicLevel, nil
	default:
		return InfoLevel, fmt.Errorf("invalid log level: %s", levelStr)
	}
}

// New creates a new Logger instance with the specified configuration
func New(config Config) *Logger {
	// Set output destination
	output := config.Output
	if output == nil {
		if config.Quiet {
			output = io.Discard
		} else {
			output = os.Stdout
		}
	}

	// Set log level
	level, err := ParseLevel(config.Level)
	if err != nil {
		level = InfoLevel
	}

	// Set formatter
	var formatter Formatter
	switch strings.ToLower(config.Format) {
	case "json":
		formatter = &JSONFormatter{}
	default:
		formatter = &TextFormatter{}
	}

	return &Logger{
		level:     level,
		output:    output,
		component: config.Component,
		formatter: formatter,
	}
}

// WithComponent creates a new logger instance with a specific component name
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		level:     l.level,
		output:    l.output,
		component: component,
		formatter: l.formatter,
	}
}

// log performs the actual logging operation
func (l *Logger) log(level Level, msg string, fields Fields) {
	if level < l.level {
		return
	}

	formatted := l.formatter.Format(level, msg, fields, l.component)
	l.output.Write([]byte(formatted))

	// Handle fatal and panic levels
	if level == FatalLevel {
		os.Exit(1)
	} else if level == PanicLevel {
		panic(msg)
	}
}

// Trace logs a trace-level message with optional fields
func (l *Logger) Trace(msg string, fields ...Fields) {
	l.log(TraceLevel, msg, l.combineFields(fields...))
}

// Debug logs a debug-level message with optional fields
func (l *Logger) Debug(msg string, fields ...Fields) {
	l.log(DebugLevel, msg, l.combineFields(fields...))
}

// Info logs an info-level message with optional fields
func (l *Logger) Info(msg string, fields ...Fields) {
	l.log(InfoLevel, msg, l.combineFields(fields...))
}

// Warn logs a warning-level message with optional fields
func (l *Logger) Warn(msg string, fields ...Fields) {
	l.log(WarnLevel, msg, l.combineFields(fields...))
}

// Error logs an error-level message with optional fields
func (l *Logger) Error(msg string, fields ...Fields) {
	l.log(ErrorLevel, msg, l.combineFields(fields...))
}

// Fatal logs a fatal-level message with optional fields and exits
func (l *Logger) Fatal(msg string, fields ...Fields) {
	l.log(FatalLevel, msg, l.combineFields(fields...))
}

// Panic logs a panic-level message with optional fields and panics
func (l *Logger) Panic(msg string, fields ...Fields) {
	l.log(PanicLevel, msg, l.combineFields(fields...))
}

// combineFields merges multiple Fields maps into a single Fields map
func (l *Logger) combineFields(fields ...Fields) Fields {
	combined := make(Fields)
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			combined[k] = v
		}
	}
	return combined
}

// Connection logging helpers for network operations
type ConnectionLogger struct {
	logger     *Logger
	remoteAddr string
	connID     string
}

// NewConnectionLogger creates a logger specifically for connection tracking
func NewConnectionLogger(logger *Logger, remoteAddr, connID string) *ConnectionLogger {
	return &ConnectionLogger{
		logger:     logger.WithComponent("connection"),
		remoteAddr: remoteAddr,
		connID:     connID,
	}
}

// Debug logs a debug message with connection context
func (cl *ConnectionLogger) Debug(msg string, fields ...Fields) {
	connFields := Fields{
		"remote_addr":   cl.remoteAddr,
		"connection_id": cl.connID,
	}
	allFields := append([]Fields{connFields}, fields...)
	cl.logger.Debug(msg, allFields...)
}

// Info logs an info message with connection context
func (cl *ConnectionLogger) Info(msg string, fields ...Fields) {
	connFields := Fields{
		"remote_addr":   cl.remoteAddr,
		"connection_id": cl.connID,
	}
	allFields := append([]Fields{connFields}, fields...)
	cl.logger.Info(msg, allFields...)
}

// Warn logs a warning message with connection context
func (cl *ConnectionLogger) Warn(msg string, fields ...Fields) {
	connFields := Fields{
		"remote_addr":   cl.remoteAddr,
		"connection_id": cl.connID,
	}
	allFields := append([]Fields{connFields}, fields...)
	cl.logger.Warn(msg, allFields...)
}

// Error logs an error message with connection context
func (cl *ConnectionLogger) Error(msg string, fields ...Fields) {
	connFields := Fields{
		"remote_addr":   cl.remoteAddr,
		"connection_id": cl.connID,
	}
	allFields := append([]Fields{connFields}, fields...)
	cl.logger.Error(msg, allFields...)
}

// Default logger instance for package-level logging
var defaultLogger *Logger

// init initializes the default logger
func init() {
	defaultLogger = New(Config{
		Level:     "info",
		Format:    "text",
		Component: "revsocks",
	})
}

// SetDefault sets the default logger instance
func SetDefault(logger *Logger) {
	defaultLogger = logger
}

// GetDefault returns the default logger instance
func GetDefault() *Logger {
	return defaultLogger
}

// Package-level logging functions that use the default logger

// Debug logs a debug message using the default logger
func Debug(msg string, fields ...Fields) {
	defaultLogger.Debug(msg, fields...)
}

// Info logs an info message using the default logger
func Info(msg string, fields ...Fields) {
	defaultLogger.Info(msg, fields...)
}

// Warn logs a warning message using the default logger
func Warn(msg string, fields ...Fields) {
	defaultLogger.Warn(msg, fields...)
}

// Error logs an error message using the default logger
func Error(msg string, fields ...Fields) {
	defaultLogger.Error(msg, fields...)
}

// Fatal logs a fatal message using the default logger and exits
func Fatal(msg string, fields ...Fields) {
	defaultLogger.Fatal(msg, fields...)
}

// Backward compatibility with standard log package
// StdLogger wraps our logger to implement the standard log.Logger interface
type StdLogger struct {
	logger *Logger
}

// NewStdLogger creates a new standard logger wrapper
func NewStdLogger(logger *Logger) *StdLogger {
	return &StdLogger{logger: logger}
}

// Write implements io.Writer for compatibility with log.SetOutput
func (sl *StdLogger) Write(p []byte) (n int, err error) {
	msg := strings.TrimSuffix(string(p), "\n")
	sl.logger.Info(msg)
	return len(p), nil
}

// Print prints a message (compatibility with log.Logger)
func (sl *StdLogger) Print(v ...interface{}) {
	sl.logger.Info(fmt.Sprint(v...))
}

// Printf prints a formatted message (compatibility with log.Logger)
func (sl *StdLogger) Printf(format string, v ...interface{}) {
	sl.logger.Info(fmt.Sprintf(format, v...))
}

// Println prints a message with newline (compatibility with log.Logger)
func (sl *StdLogger) Println(v ...interface{}) {
	sl.logger.Info(fmt.Sprintln(v...))
}

// ReplaceStandardLogger replaces the standard library logger with our logger
func ReplaceStandardLogger(logger *Logger) {
	stdLogger := NewStdLogger(logger)
	log.SetOutput(stdLogger)
	log.SetFlags(0) // Disable standard log formatting since we handle it
}
