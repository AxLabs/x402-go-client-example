// Package signer is a thin wrapper around the x402 SDK's EVM client signer.
//
// All actual cryptographic signing (EIP-712 / EIP-3009 typed data) is
// performed by the SDK (github.com/x402-foundation/x402/go/signers/evm);
// this package only adapts it to a small, test-friendly interface used by
// the CLI and wires it into [x402adapter.EVMSigner].
package signer

import (
	"errors"

	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
)

// Signer is the minimal contract the CLI needs: a human-readable address
// for logging and the underlying SDK signer to hand to the x402 adapter.
type Signer interface {
	Address() string
	EVMSigner() x402adapter.EVMSigner
}

// EthereumSigner wraps an SDK EVM signer built from a hex-encoded private
// key. It does not perform any signing itself.
type EthereumSigner struct {
	inner x402adapter.EVMSigner
	addr  string
}

// NewEthereumSigner builds an EthereumSigner from a hex-encoded private key.
// The private key never leaves the SDK after this call.
func NewEthereumSigner(privateKeyHex string) (*EthereumSigner, error) {
	if privateKeyHex == "" {
		return nil, errors.New("private key cannot be empty")
	}
	s, err := x402adapter.NewEVMSignerFromPrivateKey(privateKeyHex)
	if err != nil {
		return nil, err
	}
	return &EthereumSigner{inner: s, addr: s.Address()}, nil
}

// Address returns the signer's Ethereum address in EIP-55 checksum form
// (provided by the SDK).
func (s *EthereumSigner) Address() string { return s.addr }

// EVMSigner exposes the underlying SDK signer so it can be registered with
// an [x402adapter.Adapter].
func (s *EthereumSigner) EVMSigner() x402adapter.EVMSigner { return s.inner }

// MockSigner is a test double that reports a fixed address and carries no
// SDK signer. It is useful for test paths that never actually trigger the
// SDK's signing machinery (dry-run, no-pay, policy-reject).
type MockSigner struct {
	addr string
}

// NewMockSigner returns a MockSigner with the given address.
func NewMockSigner(addr string) *MockSigner { return &MockSigner{addr: addr} }

// Address returns the mock address.
func (m *MockSigner) Address() string { return m.addr }

// EVMSigner returns nil; MockSigner does not drive real SDK signing.
func (m *MockSigner) EVMSigner() x402adapter.EVMSigner { return nil }
