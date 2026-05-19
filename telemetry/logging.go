package telemetry

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// LogLevel represents the logging level.
type LogLevel string

const (
	// LevelDebug represents debug log level.
	LevelDebug LogLevel = "debug"
	// LevelInfo represents info log level.
	LevelInfo LogLevel = "info"
	// LevelWarn represents warn log level.
	LevelWarn LogLevel = "warn"
	// LevelError represents error log level.
	LevelError LogLevel = "error"
)

// DefaultLogger is the global logger instance.
var DefaultLogger *slog.Logger

func init() {
	// Initialize with default settings
	DefaultLogger = NewLogger(string(LevelInfo))
}

// NewLogger creates a new slog.Logger with JSON handler and configurable level.
func NewLogger(level string) *slog.Logger {
	// Create JSON handler to stdout
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	})

	return slog.New(handler)
}

// NewLoggerWithOutput creates a new slog.Logger with custom output writer.
func NewLoggerWithOutput(level string, output io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: parseLevel(level),
	})

	return slog.New(handler)
}

// parseLevel converts string level to slog.Level.
func parseLevel(level string) slog.Level {
	switch level {
	case string(LevelDebug):
		return slog.LevelDebug
	case string(LevelInfo):
		return slog.LevelInfo
	case string(LevelWarn):
		return slog.LevelWarn
	case string(LevelError):
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithRequestID returns a logger with request ID attached.
func WithRequestID(ctx context.Context, logger *slog.Logger) *slog.Logger {
	reqID := GetRequestID(ctx)
	if reqID != "" {
		return logger.With("req_id", reqID)
	}
	return logger
}

// LogCtx returns a logger from context with request ID if available.
func LogCtx(ctx context.Context) *slog.Logger {
	logger := DefaultLogger
	reqID := GetRequestID(ctx)
	if reqID != "" {
		logger = logger.With("req_id", reqID)
	}
	return logger
}

// Debug logs a debug message.
func Debug(msg string, args ...interface{}) {
	DefaultLogger.Debug(msg, args...)
}

// Info logs an info message.
func Info(msg string, args ...interface{}) {
	DefaultLogger.Info(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...interface{}) {
	DefaultLogger.Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...interface{}) {
	DefaultLogger.Error(msg, args...)
}

// SetLogger sets the global default logger.
func SetLogger(logger *slog.Logger) {
	DefaultLogger = logger
}