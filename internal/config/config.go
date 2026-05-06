// Package config handles application configuration from environment variables and flags.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all client configuration.
type Config struct {
	// Logging
	LogLevel string `json:"logLevel"`
	LogJSON  bool   `json:"logJson"`

	// Signer/Wallet
	PrivateKey string `json:"-"` // Never serialize the private key

	// Payment Policy
	MaxAmount      string   `json:"maxAmount"`      // Maximum payment amount (in smallest unit, e.g., wei)
	AllowedAssets  []string `json:"allowedAssets"`  // Allowed asset types (e.g., "USDC", "ETH")
	AllowedChainID string   `json:"allowedChainId"` // Allowed chain/network ID
	AllowedPayTo   []string `json:"allowedPayTo"`   // Allowed payment recipient addresses

	// Option Selection Preferences
	PreferredNetworks        []string `json:"preferredNetworks"`        // CAIP-2 network preferences (e.g., "eip155:84532")
	PreferredAssets          []string `json:"preferredAssets"`          // Asset address preferences
	PreferredTransferMethods []string `json:"preferredTransferMethods"` // Transfer method preferences (eip3009, permit2)
	SelectionStrategy        string   `json:"selectionStrategy"`        // "server-order" or "preference-first"

	// HTTP Client
	Timeout time.Duration `json:"timeout"`

	// Request behavior
	DryRun bool `json:"dryRun"` // Parse 402 but don't pay
	NoPay  bool `json:"noPay"`  // Don't attempt payment flow
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		LogLevel:                 "info",
		LogJSON:                  false,
		MaxAmount:                "1000000", // Default max: 1 USDC (6 decimals)
		AllowedAssets:            []string{},
		AllowedChainID:           "",
		AllowedPayTo:             []string{},
		PreferredNetworks:        []string{},
		PreferredAssets:          []string{},
		PreferredTransferMethods: []string{},
		SelectionStrategy:        "server-order",
		Timeout:                  30 * time.Second,
		DryRun:                   false,
		NoPay:                    false,
	}
}

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() (*Config, error) {
	cfg := DefaultConfig()

	if v := os.Getenv("CLIENT_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if v := os.Getenv("CLIENT_LOG_JSON"); v != "" {
		cfg.LogJSON = parseBool(v)
	}

	if v := os.Getenv("CLIENT_PRIVATE_KEY"); v != "" {
		cfg.PrivateKey = v
	}

	if v := os.Getenv("CLIENT_MAX_AMOUNT"); v != "" {
		cfg.MaxAmount = v
	}

	if v := os.Getenv("CLIENT_ALLOWED_ASSET"); v != "" {
		cfg.AllowedAssets = parseList(v)
	}

	if v := os.Getenv("CLIENT_ALLOWED_CHAIN_ID"); v != "" {
		cfg.AllowedChainID = NormalizeToCaip2(v)
	}

	if v := os.Getenv("CLIENT_ALLOWED_PAY_TO"); v != "" {
		cfg.AllowedPayTo = parseList(v)
	}

	if v := os.Getenv("CLIENT_PREFERRED_NETWORKS"); v != "" {
		cfg.PreferredNetworks = normalizeNetworks(parseList(v))
	}

	if v := os.Getenv("CLIENT_PREFERRED_ASSETS"); v != "" {
		cfg.PreferredAssets = parseList(v)
	}

	if v := os.Getenv("CLIENT_PREFERRED_TRANSFER_METHODS"); v != "" {
		cfg.PreferredTransferMethods = parseList(v)
	}

	if v := os.Getenv("CLIENT_SELECTION_STRATEGY"); v != "" {
		cfg.SelectionStrategy = strings.ToLower(strings.TrimSpace(v))
	}

	if v := os.Getenv("CLIENT_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid CLIENT_TIMEOUT: %w", err)
		}
		cfg.Timeout = d
	}

	if v := os.Getenv("CLIENT_DRY_RUN"); v != "" {
		cfg.DryRun = parseBool(v)
	}

	if v := os.Getenv("CLIENT_NO_PAY"); v != "" {
		cfg.NoPay = parseBool(v)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	var errs []error

	// Validate max amount is a valid number
	if c.MaxAmount != "" {
		if _, err := strconv.ParseUint(c.MaxAmount, 10, 64); err != nil {
			errs = append(errs, fmt.Errorf("invalid MaxAmount %q: must be a positive integer", c.MaxAmount))
		}
	}

	// Validate log level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.LogLevel)] {
		errs = append(errs, fmt.Errorf("invalid LogLevel %q: must be one of debug, info, warn, error", c.LogLevel))
	}

	// Validate timeout
	if c.Timeout <= 0 {
		errs = append(errs, errors.New("Timeout must be positive"))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// HasPrivateKey returns true if a private key is configured.
func (c *Config) HasPrivateKey() bool {
	return c.PrivateKey != ""
}

// GetMaxAmountUint64 returns MaxAmount as uint64, or 0 on error.
func (c *Config) GetMaxAmountUint64() uint64 {
	if c.MaxAmount == "" {
		return 0
	}
	v, _ := strconv.ParseUint(c.MaxAmount, 10, 64)
	return v
}

// parseBool parses a boolean string, accepting various common formats.
func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// parseList parses a comma-separated list into a slice.
func parseList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// normalizeNetworks normalizes network identifiers into CAIP-2 format.
// Bare numeric chain IDs (e.g. "84532") are converted to "eip155:84532".
// Already-qualified identifiers are left unchanged.
func normalizeNetworks(networks []string) []string {
	out := make([]string, 0, len(networks))
	for _, n := range networks {
		out = append(out, NormalizeToCaip2(n))
	}
	return out
}

// NormalizeToCaip2 converts a network identifier to CAIP-2 format.
// If the input is a bare numeric string (e.g., "84532"), it becomes "eip155:84532".
// If it already contains a colon (e.g., "eip155:84532"), it's returned as-is.
func NormalizeToCaip2(network string) string {
	network = strings.TrimSpace(network)
	if network == "" {
		return network
	}
	// If already in CAIP-2 format (contains ':'), leave as-is.
	if strings.Contains(network, ":") {
		return network
	}
	// If purely numeric, assume EIP-155 (EVM).
	if _, err := strconv.ParseUint(network, 10, 64); err == nil {
		return "eip155:" + network
	}
	// Otherwise return as-is (unknown namespace).
	return network
}
