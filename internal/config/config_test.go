package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.LogLevel != "info" {
		t.Errorf("Default LogLevel = %q, want %q", cfg.LogLevel, "info")
	}

	if cfg.Timeout != 30*time.Second {
		t.Errorf("Default Timeout = %v, want %v", cfg.Timeout, 30*time.Second)
	}

	if cfg.DryRun {
		t.Error("Default DryRun should be false")
	}

	if cfg.NoPay {
		t.Error("Default NoPay should be false")
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set up test environment
	os.Setenv("CLIENT_LOG_LEVEL", "debug")
	os.Setenv("CLIENT_MAX_AMOUNT", "5000000")
	os.Setenv("CLIENT_ALLOWED_ASSET", "USDC,ETH")
	os.Setenv("CLIENT_ALLOWED_CHAIN_ID", "84532")
	os.Setenv("CLIENT_TIMEOUT", "60s")
	os.Setenv("CLIENT_DRY_RUN", "true")

	defer func() {
		os.Unsetenv("CLIENT_LOG_LEVEL")
		os.Unsetenv("CLIENT_MAX_AMOUNT")
		os.Unsetenv("CLIENT_ALLOWED_ASSET")
		os.Unsetenv("CLIENT_ALLOWED_CHAIN_ID")
		os.Unsetenv("CLIENT_TIMEOUT")
		os.Unsetenv("CLIENT_DRY_RUN")
	}()

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}

	if cfg.MaxAmount != "5000000" {
		t.Errorf("MaxAmount = %q, want %q", cfg.MaxAmount, "5000000")
	}

	if len(cfg.AllowedAssets) != 2 {
		t.Errorf("AllowedAssets length = %d, want 2", len(cfg.AllowedAssets))
	}

	if cfg.AllowedChainID != "eip155:84532" {
		t.Errorf("AllowedChainID = %q, want %q", cfg.AllowedChainID, "eip155:84532")
	}

	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 60*time.Second)
	}

	if !cfg.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid log level",
			config: &Config{
				LogLevel: "invalid",
				Timeout:  30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid max amount",
			config: &Config{
				LogLevel:  "info",
				MaxAmount: "not-a-number",
				Timeout:   30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid timeout",
			config: &Config{
				LogLevel: "info",
				Timeout:  0,
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			config: &Config{
				LogLevel: "info",
				Timeout:  -1 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHasPrivateKey(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HasPrivateKey() {
		t.Error("HasPrivateKey() should return false when not set")
	}

	cfg.PrivateKey = "0xabc123"
	if !cfg.HasPrivateKey() {
		t.Error("HasPrivateKey() should return true when set")
	}
}

func TestGetMaxAmountUint64(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAmount = "1000000"

	result := cfg.GetMaxAmountUint64()
	if result != 1000000 {
		t.Errorf("GetMaxAmountUint64() = %d, want %d", result, 1000000)
	}

	cfg.MaxAmount = ""
	result = cfg.GetMaxAmountUint64()
	if result != 0 {
		t.Errorf("GetMaxAmountUint64() with empty string = %d, want 0", result)
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{"0", false},
		{"false", false},
		{"no", false},
		{"off", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseBool(tt.input)
			if result != tt.expected {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeToCaip2(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"84532", "eip155:84532"},
		{"eip155:84532", "eip155:84532"},
		{"1", "eip155:1"},
		{"", ""},
		{"eip155:1", "eip155:1"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeToCaip2(tt.input); got != tt.expected {
				t.Errorf("NormalizeToCaip2(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b, c", []string{"a", "b", "c"}},
		{"  a  ,  b  ", []string{"a", "b"}},
		{",a,,b,", []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseList(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseList(%q) length = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("parseList(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}
