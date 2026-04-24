package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"unknown", LevelInfo},
		{"", LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLevelToSlogLevel(t *testing.T) {
	tests := []struct {
		level    Level
		expected slog.Level
	}{
		{LevelDebug, slog.LevelDebug},
		{LevelInfo, slog.LevelInfo},
		{LevelWarn, slog.LevelWarn},
		{LevelError, slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			result := tt.level.ToSlogLevel()
			if result != tt.expected {
				t.Errorf("Level(%q).ToSlogLevel() = %v, want %v", tt.level, result, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	var buf bytes.Buffer

	logger := New(Options{
		Level:  LevelDebug,
		Output: &buf,
		JSON:   false,
	})

	if logger == nil {
		t.Fatal("New() returned nil")
	}

	if logger.Level() != LevelDebug {
		t.Errorf("logger.Level() = %v, want %v", logger.Level(), LevelDebug)
	}

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Log output should contain message, got: %s", output)
	}
}

func TestNewJSON(t *testing.T) {
	var buf bytes.Buffer

	logger := New(Options{
		Level:  LevelInfo,
		Output: &buf,
		JSON:   true,
	})

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, `"msg"`) {
		t.Errorf("JSON output should contain msg field, got: %s", output)
	}
}

func TestWithComponent(t *testing.T) {
	var buf bytes.Buffer

	logger := New(Options{
		Level:  LevelInfo,
		Output: &buf,
	})

	componentLogger := logger.WithComponent("http")
	componentLogger.Info("request received")

	output := buf.String()
	if !strings.Contains(output, "http") {
		t.Errorf("Output should contain component name, got: %s", output)
	}
}

func TestIsDebug(t *testing.T) {
	debugLogger := New(Options{Level: LevelDebug})
	if !debugLogger.IsDebug() {
		t.Error("IsDebug() should return true for debug level")
	}

	infoLogger := New(Options{Level: LevelInfo})
	if infoLogger.IsDebug() {
		t.Error("IsDebug() should return false for info level")
	}
}
