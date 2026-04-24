package policy

import (
	"testing"

	"github.com/bane-labs-org/x402-buyer-client-go/internal/x402adapter"
)

func reqs(overrides func(r *x402adapter.Requirements)) *x402adapter.Requirements {
	r := &x402adapter.Requirements{
		Scheme:            "exact",
		Network:           "eip155:84532",
		Asset:             "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
		Amount:            "100000",
		PayTo:             "0x1111111111111111111111111111111111111111",
		MaxTimeoutSeconds: 300,
	}
	if overrides != nil {
		overrides(r)
	}
	return r
}

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if p.MaxAmount != 0 {
		t.Errorf("MaxAmount = %d, want 0", p.MaxAmount)
	}
	if len(p.AllowedSchemes) != 1 || p.AllowedSchemes[0] != "exact" {
		t.Errorf("AllowedSchemes = %v, want [exact]", p.AllowedSchemes)
	}
	if err := p.Validate(reqs(nil)); err != nil {
		t.Errorf("DefaultPolicy should accept a typical request, got %v", err)
	}
}

func TestValidate_Nil(t *testing.T) {
	if err := DefaultPolicy().Validate(nil); err == nil {
		t.Error("Validate(nil) should error")
	}
}

func TestValidate_Amount(t *testing.T) {
	p := &Policy{MaxAmount: 1000, AllowedSchemes: []string{"exact"}}

	if err := p.Validate(reqs(func(r *x402adapter.Requirements) { r.Amount = "500" })); err != nil {
		t.Errorf("under-limit should pass, got %v", err)
	}
	if err := p.Validate(reqs(func(r *x402adapter.Requirements) { r.Amount = "5000" })); err == nil {
		t.Error("over-limit should fail")
	}
	if err := p.Validate(reqs(func(r *x402adapter.Requirements) { r.Amount = "notanumber" })); err == nil {
		t.Error("non-numeric amount should fail")
	}
}

func TestValidate_Scheme(t *testing.T) {
	p := &Policy{AllowedSchemes: []string{"exact"}}
	if err := p.Validate(reqs(func(r *x402adapter.Requirements) { r.Scheme = "upto" })); err == nil {
		t.Error("unknown scheme should be rejected")
	}
}

func TestValidate_Network(t *testing.T) {
	p := &Policy{AllowedChainIDs: []string{"eip155:84532"}, AllowedSchemes: []string{"exact"}}
	if err := p.Validate(reqs(nil)); err != nil {
		t.Errorf("allowed network should pass, got %v", err)
	}
	if err := p.Validate(reqs(func(r *x402adapter.Requirements) { r.Network = "eip155:137" })); err == nil {
		t.Error("disallowed network should fail")
	}
}

func TestValidate_PayTo(t *testing.T) {
	p := &Policy{AllowedPayTo: []string{"0xAAaa"}, AllowedSchemes: []string{"exact"}}
	if err := p.Validate(reqs(func(r *x402adapter.Requirements) { r.PayTo = "0xaaaa" })); err != nil {
		t.Errorf("case-insensitive match should pass, got %v", err)
	}
	if err := p.Validate(reqs(func(r *x402adapter.Requirements) { r.PayTo = "0xbbbb" })); err == nil {
		t.Error("unknown payTo should fail")
	}
}

func TestValidate_Asset(t *testing.T) {
	p := &Policy{
		AllowedAssets:  []string{"0x036CbD53842c5426634e7929541eC2318f3dCF7e"},
		AllowedSchemes: []string{"exact"},
	}
	if err := p.Validate(reqs(nil)); err != nil {
		t.Errorf("allowed asset should pass, got %v", err)
	}
	if err := p.Validate(reqs(func(r *x402adapter.Requirements) {
		r.Asset = "0x0000000000000000000000000000000000000000"
	})); err == nil {
		t.Error("unknown asset should fail")
	}
}

func TestValidate_MultipleViolations(t *testing.T) {
	p := &Policy{
		MaxAmount:       100,
		AllowedChainIDs: []string{"eip155:1"},
		AllowedPayTo:    []string{"0xAllowed"},
		AllowedSchemes:  []string{"exact"},
	}
	err := p.Validate(reqs(func(r *x402adapter.Requirements) {
		r.Network = "eip155:137"
		r.Amount = "1000"
		r.PayTo = "0xNotAllowed"
	}))
	if err == nil {
		t.Fatal("expected violations")
	}
	vs, ok := err.(PolicyViolations)
	if !ok {
		t.Fatalf("expected PolicyViolations, got %T", err)
	}
	if len(vs) < 3 {
		t.Errorf("expected >=3 violations, got %d: %v", len(vs), vs)
	}
}

func TestNewPolicyFromConfig(t *testing.T) {
	p := NewPolicyFromConfig(1000000, []string{"USDC"}, []string{"eip155:84532"}, []string{"0x1234"})
	if p.MaxAmount != 1000000 {
		t.Errorf("MaxAmount = %d, want 1000000", p.MaxAmount)
	}
	if p.String() == "" {
		t.Error("String() should not be empty")
	}
}
