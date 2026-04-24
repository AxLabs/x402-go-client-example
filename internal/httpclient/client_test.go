package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bane-labs-org/x402-buyer-client-go/internal/logging"
	"github.com/bane-labs-org/x402-buyer-client-go/internal/payment/policy"
	"github.com/bane-labs-org/x402-buyer-client-go/internal/x402adapter"
)

// v1Body is the legacy 402 body shape the SDK's HTTPClient parses.
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

func newV1Body() v1Body {
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

func TestNew_Defaults(t *testing.T) {
	c := New(DefaultOptions())
	if c == nil || c.policy == nil || c.logger == nil {
		t.Fatal("New() did not populate defaults")
	}
}

func TestClient_NoPaymentRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(Options{Timeout: 5 * time.Second, Logger: debugLogger()})
	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if res.PaymentRequired || res.PaymentMade {
		t.Errorf("flags should be false, got PaymentRequired=%v PaymentMade=%v", res.PaymentRequired, res.PaymentMade)
	}
}

func TestClient_PaymentRequired_NoPay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(newV1Body())
	}))
	defer srv.Close()

	adapter := x402adapter.NewForEVM(nil)
	c := New(Options{
		Timeout: 5 * time.Second,
		Adapter: adapter,
		Logger:  debugLogger(),
		NoPay:   true,
	})
	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if !res.PaymentRequired {
		t.Error("PaymentRequired should be true")
	}
	if res.Requirements == nil {
		t.Fatal("Requirements should be parsed by the SDK")
	}
	if res.Requirements.Scheme != "exact" {
		t.Errorf("Requirements.Scheme = %q, want exact", res.Requirements.Scheme)
	}
	if res.PaymentMade || res.Retried {
		t.Error("no-pay: PaymentMade/Retried must stay false")
	}
}

func TestClient_PolicyRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(newV1Body())
	}))
	defer srv.Close()

	adapter := x402adapter.NewForEVM(nil)
	strict := &policy.Policy{MaxAmount: 1, AllowedSchemes: []string{"exact"}}

	c := New(Options{
		Timeout: 5 * time.Second,
		Adapter: adapter,
		Policy:  strict,
		Logger:  debugLogger(),
	})
	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected policy rejection error")
	}
}

func TestClient_DryRun_NoRetry(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(newV1Body())
	}))
	defer srv.Close()

	adapter := x402adapter.NewForEVM(nil)
	c := New(Options{
		Timeout: 5 * time.Second,
		Adapter: adapter,
		Logger:  debugLogger(),
		DryRun:  true,
	})
	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if count != 1 {
		t.Errorf("dry-run must not retry; got %d requests", count)
	}
	if res.Retried || res.PaymentMade {
		t.Error("dry-run: Retried/PaymentMade must stay false")
	}
	if res.Requirements == nil {
		t.Error("dry-run should still surface Requirements")
	}
}

func TestFormatRequirements(t *testing.T) {
	r := &x402adapter.Requirements{
		Scheme: "exact", Network: "eip155:84532", Amount: "100000",
		Asset: "0xAbC", PayTo: "0xDEF", MaxTimeoutSeconds: 300,
	}
	out := FormatRequirements(r)
	for _, want := range []string{"exact", "eip155:84532", "100000", "0xAbC", "0xDEF", "300"} {
		if !contains(out, want) {
			t.Errorf("FormatRequirements() missing %q in: %s", want, out)
		}
	}
	if FormatRequirements(nil) == "" {
		t.Error("FormatRequirements(nil) should not be empty")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
