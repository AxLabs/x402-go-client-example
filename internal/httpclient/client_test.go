package httpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bane-labs-org/x402-go-client-example/internal/logging"
	"github.com/bane-labs-org/x402-go-client-example/internal/payment/policy"
	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
	x402types "github.com/x402-foundation/x402/go/types"
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

func TestClient_RetrySurfacesSecond402HeaderError(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			w.WriteHeader(http.StatusPaymentRequired)
			_ = json.NewEncoder(w).Encode(newV1Body())
			return
		}

		pr := x402types.PaymentRequired{
			X402Version: 2,
			Error:       "invalid_exact_evm_missing_eip712_domain: missing EIP-712 domain name/version in requirements.extra",
			Accepts: []x402types.PaymentRequirements{{
				Scheme:  "exact",
				Network: "eip155:12227332",
				Asset:   "0xD4ac6B385C16cd94A8E54aB422138833804AE443",
				Amount:  "20000000000000000",
				PayTo:   "0xE2E3EecCb2C3f9701Db514bb2a64bA63646E9055",
				Extra: map[string]interface{}{
					"assetTransferMethod": "eip3009",
				},
			}},
		}
		encoded, err := encodePaymentRequiredHeader(pr)
		if err != nil {
			t.Fatalf("encodePaymentRequiredHeader() err = %v", err)
		}
		w.Header().Set("PAYMENT-REQUIRED", encoded)
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	signer, err := x402adapter.NewEVMSignerFromPrivateKey("0x4c0883a69102937d6231471b5dbb6204fe5129617082792aeef6f9f6f7f8f62d")
	if err != nil {
		t.Fatalf("NewEVMSignerFromPrivateKey() err = %v", err)
	}

	c := New(Options{
		Timeout: 5 * time.Second,
		Adapter: x402adapter.NewForEVM(signer),
		Logger:  debugLogger(),
	})
	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get() err = %v", err)
	}
	if !res.Retried || !res.PaymentMade {
		t.Fatalf("expected retry path, got Retried=%v PaymentMade=%v", res.Retried, res.PaymentMade)
	}
	want := "invalid_exact_evm_missing_eip712_domain: missing EIP-712 domain name/version in requirements.extra"
	if res.RetryError != want {
		t.Fatalf("RetryError = %q, want %q", res.RetryError, want)
	}
	if got := FormatResult(res); !contains(got, want) {
		t.Fatalf("FormatResult() missing retry error: %s", got)
	}
}

func encodePaymentRequiredHeader(pr x402types.PaymentRequired) (string, error) {
	data, err := json.Marshal(pr)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
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
