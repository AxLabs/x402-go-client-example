package version

import (
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	info := Get()

	if info.Version == "" {
		t.Error("Version should not be empty")
	}

	if info.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}

	if info.OS == "" {
		t.Error("OS should not be empty")
	}

	if info.Arch == "" {
		t.Error("Arch should not be empty")
	}
}

func TestInfoString(t *testing.T) {
	info := Info{
		Version:   "1.0.0",
		Commit:    "abc123",
		BuildTime: "2024-01-01T00:00:00Z",
		GoVersion: "go1.22.0",
		OS:        "linux",
		Arch:      "amd64",
	}

	result := info.String()

	if !strings.Contains(result, "1.0.0") {
		t.Errorf("String() should contain version, got: %s", result)
	}

	if !strings.Contains(result, "abc123") {
		t.Errorf("String() should contain commit, got: %s", result)
	}
}

func TestInfoShort(t *testing.T) {
	info := Info{
		Version: "1.0.0",
	}

	result := info.Short()

	if result != "x402-go-client-example 1.0.0" {
		t.Errorf("Short() = %q, want %q", result, "x402-go-client-example 1.0.0")
	}
}
