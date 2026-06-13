package log

import (
	"log/slog"
	"os"
)

// Level constants for logging.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// InitLogger initializes the global slog logger with the specified level.
func InitLogger(level string) {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)
}

// WithComponent returns a logger with a component field.
func WithComponent(component string) *slog.Logger {
	return slog.With(slog.String("component", component))
}

// WithRequestID returns a logger with a request_id field.
func WithRequestID(requestID string) *slog.Logger {
	return slog.With(slog.String("request_id", requestID))
}

// WithModule returns a logger with module and sub-module fields.
func WithModule(module, submodule string) *slog.Logger {
	return slog.With(
		slog.String("module", module),
		slog.String("submodule", submodule),
	)
}
