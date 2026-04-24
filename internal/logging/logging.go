// Package logging provides structured logging utilities using slog.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Level represents logging level.
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// ParseLevel converts a string to a Level, defaulting to info.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// ToSlogLevel converts Level to slog.Level.
func (l Level) ToSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Logger wraps slog.Logger with additional convenience methods.
type Logger struct {
	*slog.Logger
	level Level
}

// Options for creating a new logger.
type Options struct {
	Level  Level
	Output io.Writer
	JSON   bool
}

// DefaultOptions returns sensible default logging options.
func DefaultOptions() Options {
	return Options{
		Level:  LevelInfo,
		Output: os.Stderr,
		JSON:   false,
	}
}

// New creates a new Logger with the given options.
func New(opts Options) *Logger {
	if opts.Output == nil {
		opts.Output = os.Stderr
	}

	handlerOpts := &slog.HandlerOptions{
		Level: opts.Level.ToSlogLevel(),
	}

	var handler slog.Handler
	if opts.JSON {
		handler = slog.NewJSONHandler(opts.Output, handlerOpts)
	} else {
		handler = slog.NewTextHandler(opts.Output, handlerOpts)
	}

	return &Logger{
		Logger: slog.New(handler),
		level:  opts.Level,
	}
}

// Default returns a default logger.
func Default() *Logger {
	return New(DefaultOptions())
}

// WithContext returns a logger with context values.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{
		Logger: l.Logger,
		level:  l.level,
	}
}

// With returns a logger with additional attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
		level:  l.level,
	}
}

// WithComponent returns a logger tagged with a component name.
func (l *Logger) WithComponent(name string) *Logger {
	return l.With("component", name)
}

// Level returns the current logging level.
func (l *Logger) Level() Level {
	return l.level
}

// IsDebug returns true if debug logging is enabled.
func (l *Logger) IsDebug() bool {
	return l.level == LevelDebug
}
