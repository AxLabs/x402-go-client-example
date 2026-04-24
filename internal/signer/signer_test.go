package signer

import (
	"strings"
	"testing"
)

// testPrivateKey is the well-known Hardhat account #0 key. Safe for tests only.
const testPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

const expectedAddr = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"

func TestNewEthereumSigner(t *testing.T) {
	tests := []struct {
		name       string
		privateKey string
		wantErr    bool
	}{
		{"valid without prefix", testPrivateKey, false},
		{"valid with 0x prefix", "0x" + testPrivateKey, false},
		{"empty", "", true},
		{"not hex", "not-hex", true},
		{"too short", "1234", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewEthereumSigner(tt.privateKey)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewEthereumSigner() err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && s == nil {
				t.Fatal("expected non-nil signer")
			}
		})
	}
}

func TestEthereumSigner_AddressAndSDKSigner(t *testing.T) {
	s, err := NewEthereumSigner(testPrivateKey)
	if err != nil {
		t.Fatalf("NewEthereumSigner() err = %v", err)
	}
	if !strings.EqualFold(s.Address(), expectedAddr) {
		t.Errorf("Address() = %q, want %q (case-insensitive)", s.Address(), expectedAddr)
	}
	if s.EVMSigner() == nil {
		t.Error("EVMSigner() should not be nil for a real signer")
	}
	// The SDK-reported address should match.
	if !strings.EqualFold(s.EVMSigner().Address(), s.Address()) {
		t.Errorf("SDK signer Address() = %q, want %q", s.EVMSigner().Address(), s.Address())
	}
}

func TestMockSigner(t *testing.T) {
	m := NewMockSigner("0xMock")
	if m.Address() != "0xMock" {
		t.Errorf("Address() = %q, want %q", m.Address(), "0xMock")
	}
	if m.EVMSigner() != nil {
		t.Error("MockSigner.EVMSigner() should return nil")
	}
}
