// Package test contains integration-style tests for the x402 client example.
//
// These tests spin up an httptest server that speaks the x402 wire protocol
// (v1 body form, which is the most widely interoperable shape) and exercise
// the full pipeline of parse -> select -> policy -> (SDK sign) -> retry.
package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bane-labs-org/x402-go-client-example/internal/httpclient"
	"github.com/bane-labs-org/x402-go-client-example/internal/logging"
	"github.com/bane-labs-org/x402-go-client-example/internal/payment/policy"
	"github.com/bane-labs-org/x402-go-client-example/internal/payment/selection"
	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
)

// v1Accepts mirrors the SDK's types.PaymentRequirementsV1 JSON shape.
type v1Accepts struct {
	Scheme            string `json:"scheme"`
	Network           string `json:"network"`
	MaxAmountRequired string `json:"maxAmountRequired"`
	Resource          string `json:"resource"`
	Description       string `json:"description"`
	MimeType          string `json:"mimeType"`
	PayTo             string `json:"payTo"`
	MaxTimeoutSeconds int    `json:"maxTimeoutSeconds"`
	Asset             string `json:"asset"`
}

type v1Body struct {
	X402Version int         `json:"x402Version"`
	Error       string      `json:"error"`
	Accepts     []v1Accepts `json:"accepts"`
}

func defaultV1() v1Body {
	return v1Body{
		X402Version: 1,
		Error:       "Payment Required",
		Accepts: []v1Accepts{{
			Scheme:            "exact",
			Network:           "eip155:84532",
			MaxAmountRequired: "100000",
			Resource:          "/paid/hello",
			Description:       "Test",
			MimeType:          "application/json",
			PayTo:             "0x1111111111111111111111111111111111111111",
			MaxTimeoutSeconds: 300,
			Asset:             "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
		}},
	}
}

func debugLogger() *logging.Logger {
	return logging.New(logging.Options{Level: logging.LevelDebug})
}

// TestIntegration_NoPaymentNeeded: happy-path 200 without any payment flow.
func TestIntegration_NoPaymentNeeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Free content!"})
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		Policy:  policy.DefaultPolicy(),
		Logger:  debugLogger(),
	})
	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if res.PaymentRequired || res.PaymentMade {
		t.Errorf("unexpected payment flags: %+v", res)
	}
}

// TestIntegration_402_NoPay: server returns 402, client surfaces requirements
// and stops (no signing, no retry).
func TestIntegration_402_NoPay(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(defaultV1())
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		Adapter: x402adapter.NewForEVM(nil),
		Policy:  policy.DefaultPolicy(),
		Logger:  debugLogger(),
		NoPay:   true,
	})

	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if count != 1 {
		t.Errorf("no-pay should issue a single request, got %d", count)
	}
	if !res.PaymentRequired {
		t.Error("PaymentRequired should be true")
	}
	if res.Requirements == nil {
		t.Fatal("SDK should have populated Requirements")
	}
	if res.Requirements.Scheme != "exact" {
		t.Errorf("Requirements.Scheme = %q, want exact", res.Requirements.Scheme)
	}
	if res.Requirements.PayTo != "0x1111111111111111111111111111111111111111" {
		t.Errorf("unexpected PayTo: %q", res.Requirements.PayTo)
	}
	if res.PaymentMade || res.Retried {
		t.Error("no-pay: PaymentMade/Retried must be false")
	}
}

// TestIntegration_PolicyRejection: requirements exceed policy cap; client
// refuses to sign.
func TestIntegration_PolicyRejection(t *testing.T) {
	body := defaultV1()
	body.Accepts[0].MaxAmountRequired = "10000000" // way above policy cap

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	strict := &policy.Policy{MaxAmount: 1000000, AllowedSchemes: []string{"exact"}}
	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		Adapter: x402adapter.NewForEVM(nil),
		Policy:  strict,
		Logger:  debugLogger(),
	})

	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected policy violation error, got nil")
	}
	if !strings.Contains(err.Error(), "policy") {
		t.Errorf("error should mention 'policy': %v", err)
	}
}

// TestIntegration_ChainRestriction: requirements network not in allowlist.
func TestIntegration_ChainRestriction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(defaultV1())
	}))
	defer srv.Close()

	// The requirements network is "eip155:84532". We explicitly disallow it.
	strict := &policy.Policy{
		AllowedChainIDs: []string{"eip155:1"},
		AllowedSchemes:  []string{"exact"},
	}
	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		Adapter: x402adapter.NewForEVM(nil),
		Policy:  strict,
		Logger:  debugLogger(),
	})

	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected chain restriction error, got nil")
	}
}

// TestIntegration_DryRun: all the way through policy, then stop before
// asking the SDK to sign.
func TestIntegration_DryRun(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(defaultV1())
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		Adapter: x402adapter.NewForEVM(nil),
		Policy:  policy.DefaultPolicy(),
		Logger:  debugLogger(),
		DryRun:  true,
	})
	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if count != 1 {
		t.Errorf("dry-run should not retry; got %d requests", count)
	}
	if res.Retried || res.PaymentMade {
		t.Error("dry-run: Retried/PaymentMade must be false")
	}
	if res.Requirements == nil {
		t.Error("dry-run should still surface Requirements")
	}
}

// ─── Multi-Option Integration Tests ──────────────────────────────────────────

func multiOptionV1() v1Body {
	return v1Body{
		X402Version: 1,
		Error:       "Payment Required",
		Accepts: []v1Accepts{
			{
				Scheme:            "exact",
				Network:           "eip155:84532",
				MaxAmountRequired: "10000000", // expensive: 10 USDC
				Resource:          "/paid/hello",
				Description:       "Base Sepolia USDC",
				MimeType:          "application/json",
				PayTo:             "0x1111111111111111111111111111111111111111",
				MaxTimeoutSeconds: 300,
				Asset:             "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
			},
			{
				Scheme:            "exact",
				Network:           "eip155:47763",
				MaxAmountRequired: "50000", // cheap: 0.05 units
				Resource:          "/paid/hello",
				Description:       "Neo X xGAS",
				MimeType:          "application/json",
				PayTo:             "0x2222222222222222222222222222222222222222",
				MaxTimeoutSeconds: 300,
				Asset:             "0xABCDEF0000000000000000000000000000000001",
			},
		},
	}
}

// TestIntegration_MultiOption_FirstFailsPolicy_SecondSucceeds: first option
// exceeds the max amount cap, second option is within policy and gets selected.
func TestIntegration_MultiOption_FirstFailsPolicy_SecondSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(multiOptionV1())
	}))
	defer srv.Close()

	pol := &policy.Policy{MaxAmount: 1000000, AllowedSchemes: []string{"exact"}}
	sel := selection.NewSelector(pol, selection.Preferences{}, selection.StrategyServerOrder)

	c := httpclient.New(httpclient.Options{
		Timeout:  5 * time.Second,
		Adapter:  x402adapter.NewForEVM(nil),
		Policy:   pol,
		Selector: sel,
		Logger:   debugLogger(),
		NoPay:    true, // Don't actually sign
	})

	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if !res.PaymentRequired {
		t.Error("PaymentRequired should be true")
	}
	if res.Requirements == nil {
		t.Fatal("Requirements should be populated")
	}
	// Second option (Neo X, cheap) should be selected.
	if res.Requirements.Network != "eip155:47763" {
		t.Errorf("Expected eip155:47763 to be selected, got %q", res.Requirements.Network)
	}
	if res.SelectionResult == nil {
		t.Fatal("SelectionResult should be populated")
	}
	if res.SelectionResult.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", res.SelectionResult.SelectedIndex)
	}
	if len(res.SelectionResult.Rejected) != 1 {
		t.Errorf("Expected 1 rejection, got %d", len(res.SelectionResult.Rejected))
	}
}

// TestIntegration_MultiOption_EIP3009_WithAlternate: tests preference-first
// selection where the preferred network (Neo X) is offered second but selected
// first due to preferences.
func TestIntegration_MultiOption_EIP3009_WithAlternate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		body := multiOptionV1()
		// Make both affordable.
		body.Accepts[0].MaxAmountRequired = "100000"
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	pol := policy.DefaultPolicy()
	prefs := selection.Preferences{
		Networks:        []string{"eip155:47763"},
		TransferMethods: []string{"eip3009"},
	}
	sel := selection.NewSelector(pol, prefs, selection.StrategyPreferenceFirst)

	c := httpclient.New(httpclient.Options{
		Timeout:  5 * time.Second,
		Adapter:  x402adapter.NewForEVM(nil),
		Policy:   pol,
		Selector: sel,
		Logger:   debugLogger(),
		NoPay:    true,
	})

	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if res.Requirements == nil {
		t.Fatal("Requirements should be populated")
	}
	// Preference-first: Neo X should be selected despite being second in server order.
	if res.Requirements.Network != "eip155:47763" {
		t.Errorf("Expected preferred network eip155:47763, got %q", res.Requirements.Network)
	}
}

// TestIntegration_MultiOption_NoAcceptable: all options fail policy, error
// message should be descriptive.
func TestIntegration_MultiOption_NoAcceptable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(multiOptionV1())
	}))
	defer srv.Close()

	// Policy: only allow chain eip155:1 (neither option matches).
	pol := &policy.Policy{
		AllowedChainIDs: []string{"eip155:1"},
		AllowedSchemes:  []string{"exact"},
	}
	sel := selection.NewSelector(pol, selection.Preferences{}, selection.StrategyServerOrder)

	c := httpclient.New(httpclient.Options{
		Timeout:  5 * time.Second,
		Adapter:  x402adapter.NewForEVM(nil),
		Policy:   pol,
		Selector: sel,
		Logger:   debugLogger(),
	})

	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error when no option is acceptable")
	}
	// Error should be descriptive.
	if !strings.Contains(err.Error(), "no acceptable payment option") {
		t.Errorf("error should mention 'no acceptable payment option': %v", err)
	}
	if !strings.Contains(err.Error(), "2 option(s) rejected") {
		t.Errorf("error should mention '2 option(s) rejected': %v", err)
	}
}

// TestIntegration_SingleOption_BackwardCompat: verifies that single-option
// responses still work exactly as before.
func TestIntegration_SingleOption_BackwardCompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(defaultV1())
	}))
	defer srv.Close()

	// Default selector with no preferences (server-order).
	pol := policy.DefaultPolicy()
	sel := selection.NewSelector(pol, selection.Preferences{}, selection.StrategyServerOrder)

	c := httpclient.New(httpclient.Options{
		Timeout:  5 * time.Second,
		Adapter:  x402adapter.NewForEVM(nil),
		Policy:   pol,
		Selector: sel,
		Logger:   debugLogger(),
		NoPay:    true,
	})

	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if !res.PaymentRequired {
		t.Error("PaymentRequired should be true")
	}
	if res.Requirements == nil {
		t.Fatal("Requirements should be populated")
	}
	if res.Requirements.Scheme != "exact" {
		t.Errorf("Requirements.Scheme = %q, want exact", res.Requirements.Scheme)
	}
	if res.Requirements.Network != "eip155:84532" {
		t.Errorf("Requirements.Network = %q, want eip155:84532", res.Requirements.Network)
	}
	if len(res.AllAccepts) != 1 {
		t.Errorf("AllAccepts length = %d, want 1", len(res.AllAccepts))
	}
}
