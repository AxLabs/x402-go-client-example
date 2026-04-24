// Package x402adapter is a thin wrapper around the official x402 Go SDK
// (github.com/x402-foundation/x402/go) that the rest of this app uses for all
// x402 protocol concerns.
//
// Scope of this package:
//
//   - construct an x402 SDK client pre-registered for the EVM "exact" scheme
//     on any "eip155:*" network;
//   - parse 402 Payment Required responses into SDK types;
//   - ask the SDK to create a payment payload (signed via SDK mechanisms);
//   - encode the payload into the HTTP headers expected by the x402 protocol
//     (PAYMENT-SIGNATURE / X-PAYMENT), delegated to the SDK.
//
// This wrapper intentionally contains no protocol logic of its own: it
// re-exports SDK types and forwards to SDK functions. All buyer-side policy
// lives in the caller (see internal/payment/policy).
package x402adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	x402 "github.com/x402-foundation/x402/go"
	x402http "github.com/x402-foundation/x402/go/http"
	x402evm "github.com/x402-foundation/x402/go/mechanisms/evm"
	exactevm "github.com/x402-foundation/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/x402-foundation/x402/go/signers/evm"
	x402types "github.com/x402-foundation/x402/go/types"
)

// Requirements is re-exported from the SDK so CLI/policy layers do not have
// to import the SDK directly.
type Requirements = x402types.PaymentRequirements

// PaymentRequired is re-exported from the SDK: the full parsed 402 response
// (version, accepted requirements, resource, extensions).
type PaymentRequired = x402types.PaymentRequired

// PaymentPayload is re-exported from the SDK: the signed payment object that
// is encoded into the retry headers.
type PaymentPayload = x402types.PaymentPayload

// EVMSigner is the interface an EVM signer must satisfy to produce EIP-712
// signatures for the "exact" scheme. Re-exported from the SDK.
type EVMSigner = x402evm.ClientEvmSigner

// NewEVMSignerFromPrivateKey builds an SDK EVM client signer from a
// hex-encoded private key. All signing (EIP-712 typed data / EIP-3009) is
// delegated to the SDK.
func NewEVMSignerFromPrivateKey(privateKeyHex string) (EVMSigner, error) {
	s, err := evmsigners.NewClientSignerFromPrivateKey(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("x402 sdk signer: %w", err)
	}
	return s, nil
}

// Adapter wraps the SDK's root X402Client together with its HTTP client
// helper. It is created once per CLI run and reused for all requests.
type Adapter struct {
	client     *x402.X402Client
	httpClient *x402http.HTTPClient
}

// NewForEVM builds an Adapter configured for the EVM "exact" scheme on any
// "eip155:*" network.
//
// If signer is nil, the adapter can still parse 402 responses (useful for the
// inspect / no-pay code paths) but will refuse to create payment payloads.
func NewForEVM(signer EVMSigner) *Adapter {
	c := x402.Newx402Client()
	if signer != nil {
		c.Register("eip155:*", exactevm.NewExactEvmScheme(signer, nil))
	}
	return &Adapter{
		client:     c,
		httpClient: x402http.Newx402HTTPClient(c),
	}
}

// ParsePaymentRequired extracts a PaymentRequired document from an HTTP 402
// response using the SDK's header/body decoder. It understands both the v2
// PAYMENT-REQUIRED base64 header and the legacy v1 body form.
func (a *Adapter) ParsePaymentRequired(resp *http.Response, body []byte) (PaymentRequired, error) {
	return a.httpClient.GetPaymentRequiredResponse(flattenHeaders(resp.Header), body)
}

// SelectRequirements asks the SDK to pick the first acceptable payment option
// from the server's Accepts list, based on the schemes we registered.
func (a *Adapter) SelectRequirements(pr PaymentRequired) (Requirements, error) {
	return a.client.SelectPaymentRequirements(pr.Accepts)
}

// CreateAndEncodePayment asks the SDK to build a signed payment payload for
// the given requirements and then encodes it into the HTTP headers that the
// server expects on the retry request.
//
// Signing happens entirely inside the SDK mechanism (EVM exact / EIP-3009).
// This app never assembles authorization messages by hand.
func (a *Adapter) CreateAndEncodePayment(
	ctx context.Context,
	pr PaymentRequired,
	reqs Requirements,
) (PaymentPayload, map[string]string, error) {
	payload, err := a.client.CreatePaymentPayload(ctx, reqs, pr.Resource, pr.Extensions)
	if err != nil {
		return PaymentPayload{}, nil, fmt.Errorf("sdk create payment payload: %w", err)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return payload, nil, fmt.Errorf("marshal payment payload: %w", err)
	}
	headers, err := a.httpClient.EncodePaymentSignatureHeader(payloadBytes)
	if err != nil {
		return payload, nil, fmt.Errorf("sdk encode payment header: %w", err)
	}
	return payload, headers, nil
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}
