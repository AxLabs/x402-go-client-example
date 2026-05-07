// Package e2e_test contains optional end-to-end integration tests that run
// against a real x402-go-server-example instance.
//
// These tests are skipped by default. To enable them:
//
//	X402_E2E_SERVER_URL=http://localhost:8080 go test ./test/e2e/... -count=1 -v
//
// Paid tests additionally require:
//
//	X402_E2E_PRIVATE_KEY=0x...
//
// The server must already be running and expose:
//
//	GET  /healthz     (unpaid)
//	GET  /info        (unpaid)
//	GET  /paid/hello  (paid, multi-option: Base Sepolia + Neo X)
//	POST /paid/echo   (paid)
package e2e_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bane-labs-org/x402-go-client-example/internal/httpclient"
	"github.com/bane-labs-org/x402-go-client-example/internal/logging"
	"github.com/bane-labs-org/x402-go-client-example/internal/payment/policy"
	"github.com/bane-labs-org/x402-go-client-example/internal/payment/selection"
	"github.com/bane-labs-org/x402-go-client-example/internal/signer"
	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// requireServer reads X402_E2E_SERVER_URL and skips if not set.
func requireServer(t *testing.T) string {
	t.Helper()
	url := os.Getenv("X402_E2E_SERVER_URL")
	if url == "" {
		t.Skip("set X402_E2E_SERVER_URL to run E2E tests")
	}
	return strings.TrimRight(url, "/")
}

// requireSigner reads X402_E2E_PRIVATE_KEY and skips if not set.
func requireSigner(t *testing.T) signer.Signer {
	t.Helper()
	key := os.Getenv("X402_E2E_PRIVATE_KEY")
	if key == "" {
		t.Skip("set X402_E2E_PRIVATE_KEY to run paid E2E tests")
	}
	s, err := signer.NewEthereumSigner(key)
	if err != nil {
		t.Fatalf("failed to create signer from X402_E2E_PRIVATE_KEY: %v", err)
	}
	return s
}

// clientOpts holds options for building an E2E test client.
type clientOpts struct {
	signer            signer.Signer
	preferredNetworks []string
	allowedChainIDs   []string
	maxAmount         uint64
	selectionStrategy selection.Strategy
}

// buildClient constructs an httpclient.Client suitable for E2E testing.
func buildClient(t *testing.T, opts clientOpts) *httpclient.Client {
	t.Helper()

	var evmSigner x402adapter.EVMSigner
	if opts.signer != nil {
		evmSigner = opts.signer.EVMSigner()
	}
	adapter := x402adapter.NewForEVM(evmSigner)

	maxAmt := opts.maxAmount
	if maxAmt == 0 {
		maxAmt = 10_000_000 // generous default for E2E
	}

	pol := policy.NewPolicyFromConfig(maxAmt, nil, opts.allowedChainIDs, nil)

	prefs := selection.Preferences{
		Networks: opts.preferredNetworks,
	}
	strat := opts.selectionStrategy
	if strat == "" {
		strat = selection.StrategyServerOrder
	}
	sel := selection.NewSelector(pol, prefs, strat)

	logger := logging.New(logging.Options{
		Level:  logging.ParseLevel("debug"),
		Output: os.Stderr,
	})

	return httpclient.New(httpclient.Options{
		Timeout:  60 * time.Second,
		Adapter:  adapter,
		Policy:   pol,
		Selector: sel,
		Logger:   logger,
	})
}

// ctx returns a context with a generous timeout for E2E.
func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return c
}

// assertPaymentResponse checks that the PAYMENT-RESPONSE header is present.
func assertPaymentResponse(t *testing.T, result *httpclient.RequestResult) {
	t.Helper()
	if result.Response == nil {
		t.Fatal("response is nil")
	}
	pr := result.Response.Header.Get("PAYMENT-RESPONSE")
	if pr == "" {
		// Also check lowercase variants (HTTP/2 normalizes to lowercase).
		pr = result.Response.Header.Get("payment-response")
	}
	if pr == "" {
		t.Error("expected PAYMENT-RESPONSE header in successful paid response, got none")
	} else {
		t.Logf("PAYMENT-RESPONSE header present (%d bytes)", len(pr))
	}
}

// ---------------------------------------------------------------------------
// Unpaid Endpoint Tests (only need X402_E2E_SERVER_URL)
// ---------------------------------------------------------------------------

func TestE2E_Healthz(t *testing.T) {
	baseURL := requireServer(t)

	client := buildClient(t, clientOpts{})
	result, err := client.Get(ctx(t), baseURL+"/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}

	if result.Response.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.Response.StatusCode)
	}
	if result.PaymentRequired {
		t.Error("unexpected payment challenge on /healthz")
	}
	t.Logf("GET /healthz → %d, body: %s", result.Response.StatusCode, string(result.Body))
}

func TestE2E_Info(t *testing.T) {
	baseURL := requireServer(t)

	client := buildClient(t, clientOpts{})
	result, err := client.Get(ctx(t), baseURL+"/info")
	if err != nil {
		t.Fatalf("GET /info failed: %v", err)
	}

	if result.Response.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.Response.StatusCode)
	}
	if result.PaymentRequired {
		t.Error("unexpected payment challenge on /info")
	}
	if len(result.Body) == 0 {
		t.Error("expected non-empty response body from /info")
	}

	// Verify pricing metadata is present.
	var info map[string]interface{}
	if err := json.Unmarshal(result.Body, &info); err != nil {
		t.Fatalf("failed to parse /info response as JSON: %v", err)
	}
	if _, ok := info["pricing"]; !ok {
		t.Log("WARNING: /info response does not contain 'pricing' field")
	}
	t.Logf("GET /info → %d, keys: %v", result.Response.StatusCode, mapKeys(info))
}

// ---------------------------------------------------------------------------
// Paid GET Flow (need X402_E2E_SERVER_URL + X402_E2E_PRIVATE_KEY)
// ---------------------------------------------------------------------------

func TestE2E_PaidHello(t *testing.T) {
	baseURL := requireServer(t)
	s := requireSigner(t)

	client := buildClient(t, clientOpts{signer: s})
	result, err := client.Get(ctx(t), baseURL+"/paid/hello")
	if err != nil {
		t.Fatalf("GET /paid/hello failed: %v", err)
	}

	if !result.PaymentRequired {
		t.Error("expected payment to be required for /paid/hello")
	}
	if !result.PaymentMade {
		t.Error("expected payment to be made")
	}
	if !result.Retried {
		t.Error("expected retry after payment")
	}
	if result.Response.StatusCode != 200 {
		t.Errorf("expected final status 200, got %d", result.Response.StatusCode)
	}
	if len(result.Body) == 0 {
		t.Error("expected non-empty response body")
	}

	assertPaymentResponse(t, result)

	t.Logf("GET /paid/hello → payment flow completed, final status: %d", result.Response.StatusCode)
	t.Logf("  Selected option index: %d", result.SelectionResult.SelectedIndex)
	t.Logf("  Network: %s", result.Requirements.Network)
	t.Logf("  Response body: %s", string(result.Body))
}

// ---------------------------------------------------------------------------
// Paid POST Body Preservation
// ---------------------------------------------------------------------------

func TestE2E_PaidEcho_BodyPreserved(t *testing.T) {
	baseURL := requireServer(t)
	s := requireSigner(t)

	client := buildClient(t, clientOpts{signer: s})
	body := []byte(`{"message":"hello from e2e"}`)
	result, err := client.Post(ctx(t), baseURL+"/paid/echo", body)
	if err != nil {
		t.Fatalf("POST /paid/echo failed: %v", err)
	}

	if !result.PaymentRequired {
		t.Error("expected payment to be required for /paid/echo")
	}
	if !result.PaymentMade {
		t.Error("expected payment to be made")
	}
	if result.Response.StatusCode != 200 {
		t.Errorf("expected final status 200, got %d", result.Response.StatusCode)
	}

	// Verify the response echoes the original message.
	var respBody map[string]interface{}
	if err := json.Unmarshal(result.Body, &respBody); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}

	// The server's echo handler should include the original message.
	respStr := string(result.Body)
	if !strings.Contains(respStr, "hello from e2e") {
		t.Errorf("response does not contain original message; body: %s", respStr)
	}

	assertPaymentResponse(t, result)

	t.Logf("POST /paid/echo → payment flow completed, body preserved")
	t.Logf("  Response: %s", respStr)
}

// ---------------------------------------------------------------------------
// Multi-Option Selection (Preference-Based)
// ---------------------------------------------------------------------------

func TestE2E_MultiOption_PreferenceSelection(t *testing.T) {
	baseURL := requireServer(t)
	s := requireSigner(t)

	// Prefer Neo X Testnet.
	client := buildClient(t, clientOpts{
		signer:            s,
		preferredNetworks: []string{"eip155:12227332"},
		selectionStrategy: selection.StrategyPreferenceFirst,
	})

	result, err := client.Get(ctx(t), baseURL+"/paid/hello")
	if err != nil {
		t.Fatalf("GET /paid/hello with Neo X preference failed: %v", err)
	}

	if !result.PaymentMade {
		t.Fatal("expected payment to be made")
	}

	// Verify multiple options were offered.
	if len(result.AllAccepts) < 2 {
		t.Skipf("server only offered %d option(s); need 2+ for multi-option test", len(result.AllAccepts))
	}

	// Verify Neo X was selected.
	if result.Requirements == nil {
		t.Fatal("no requirements selected")
	}
	if result.Requirements.Network != "eip155:12227332" {
		t.Errorf("expected selected network eip155:12227332 (Neo X), got %s", result.Requirements.Network)
	}

	assertPaymentResponse(t, result)

	t.Logf("Multi-option preference: selected %s (index %d) from %d options",
		result.Requirements.Network, result.SelectionResult.SelectedIndex, len(result.AllAccepts))
}

// ---------------------------------------------------------------------------
// Policy Rejection Fallback
// ---------------------------------------------------------------------------

func TestE2E_PolicyRejection_Fallback(t *testing.T) {
	baseURL := requireServer(t)
	s := requireSigner(t)

	// Restrict allowed chains to Neo X only → Base Sepolia is rejected by policy.
	client := buildClient(t, clientOpts{
		signer:          s,
		allowedChainIDs: []string{"eip155:12227332"},
	})

	result, err := client.Get(ctx(t), baseURL+"/paid/hello")
	if err != nil {
		t.Fatalf("GET /paid/hello with chain restriction failed: %v", err)
	}

	if !result.PaymentMade {
		t.Fatal("expected payment to be made (fallback to Neo X)")
	}

	// Verify that rejected options exist (Base Sepolia should be rejected).
	if result.SelectionResult == nil {
		t.Fatal("no selection result")
	}
	if len(result.SelectionResult.Rejected) == 0 {
		t.Log("WARNING: no rejected options; server may only offer Neo X")
	} else {
		for _, rej := range result.SelectionResult.Rejected {
			t.Logf("  Rejected option [%d] %s: %s", rej.Index, rej.Requirements.Network, rej.Reason)
		}
	}

	// Verify selected option is Neo X.
	if result.Requirements != nil && result.Requirements.Network != "eip155:12227332" {
		t.Errorf("expected Neo X (eip155:12227332) selected, got %s", result.Requirements.Network)
	}

	assertPaymentResponse(t, result)

	t.Logf("Policy rejection fallback: selected %s after rejecting %d other option(s)",
		result.Requirements.Network, len(result.SelectionResult.Rejected))
}

// ---------------------------------------------------------------------------
// No Acceptable Options (Explicit Error)
// ---------------------------------------------------------------------------

func TestE2E_NoAcceptableOptions(t *testing.T) {
	baseURL := requireServer(t)

	// Restrict to a network the server doesn't offer.
	client := buildClient(t, clientOpts{
		signer:          requireSigner(t),
		allowedChainIDs: []string{"eip155:99999"},
	})

	result, err := client.Get(ctx(t), baseURL+"/paid/hello")

	// Expect an error indicating no acceptable option.
	if err == nil {
		t.Fatal("expected error when no acceptable payment option exists, got nil")
	}

	if !strings.Contains(err.Error(), "no acceptable payment option") {
		t.Errorf("error message should mention 'no acceptable payment option', got: %s", err.Error())
	}

	// The error should include rejection reasons.
	if !strings.Contains(err.Error(), "rejected") || !strings.Contains(err.Error(), "eip155:") {
		t.Logf("WARNING: error may not include detailed rejection reasons: %s", err.Error())
	}

	t.Logf("No acceptable options: error = %s", err.Error())

	// Verify the result still has diagnostic info (if returned).
	if result != nil && result.SelectionResult != nil {
		t.Logf("  All offered options: %d", len(result.AllAccepts))
		t.Logf("  All rejected: %d", len(result.SelectionResult.Rejected))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
