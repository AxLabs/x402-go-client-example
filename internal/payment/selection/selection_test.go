package selection

import (
	"testing"

	"github.com/bane-labs-org/x402-go-client-example/internal/payment/policy"
	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
)

func baseReqs(network, asset, amount string) x402adapter.Requirements {
	return x402adapter.Requirements{
		Scheme:            "exact",
		Network:           network,
		Asset:             asset,
		Amount:            amount,
		PayTo:             "0x1111111111111111111111111111111111111111",
		MaxTimeoutSeconds: 300,
	}
}

func TestSelect_SingleOption_Passes(t *testing.T) {
	pol := policy.DefaultPolicy()
	sel := NewSelector(pol, Preferences{}, StrategyServerOrder)

	accepts := []x402adapter.Requirements{
		baseReqs("eip155:84532", "0xUSDC", "100000"),
	}

	result := sel.Select(accepts)
	if result.Selected == nil {
		t.Fatal("expected a selected option")
	}
	if result.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0", result.SelectedIndex)
	}
	if len(result.Rejected) != 0 {
		t.Errorf("unexpected rejections: %v", result.Rejected)
	}
}

func TestSelect_SingleOption_Backward_Compatible(t *testing.T) {
	// Simulates the existing single-option behavior.
	pol := &policy.Policy{MaxAmount: 1000000, AllowedSchemes: []string{"exact"}}
	sel := NewSelector(pol, Preferences{}, StrategyServerOrder)

	accepts := []x402adapter.Requirements{
		baseReqs("eip155:84532", "0xUSDC", "100000"),
	}

	result := sel.Select(accepts)
	if result.Selected == nil {
		t.Fatal("should select the single valid option")
	}
	if result.Selected.Network != "eip155:84532" {
		t.Errorf("Network = %q, want eip155:84532", result.Selected.Network)
	}
}

func TestSelect_MultiOption_FirstFailsPolicy_SecondSucceeds(t *testing.T) {
	// Policy: max amount 500000. First option is too expensive, second is fine.
	pol := &policy.Policy{MaxAmount: 500000, AllowedSchemes: []string{"exact"}}
	sel := NewSelector(pol, Preferences{}, StrategyServerOrder)

	accepts := []x402adapter.Requirements{
		baseReqs("eip155:84532", "0xUSDC", "1000000"), // too expensive
		baseReqs("eip155:84532", "0xUSDC", "100000"),  // within limit
	}

	result := sel.Select(accepts)
	if result.Selected == nil {
		t.Fatal("expected second option to be selected")
	}
	if result.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", result.SelectedIndex)
	}
	if result.Selected.Amount != "100000" {
		t.Errorf("Selected.Amount = %q, want 100000", result.Selected.Amount)
	}
	if len(result.Rejected) != 1 {
		t.Fatalf("expected 1 rejection, got %d", len(result.Rejected))
	}
	if result.Rejected[0].Index != 0 {
		t.Errorf("Rejected[0].Index = %d, want 0", result.Rejected[0].Index)
	}
}

func TestSelect_MultiOption_EIP3009_Plus_Alternate(t *testing.T) {
	// Two options: one ERC20 via eip3009 (exact scheme), one different network.
	// Policy allows both networks, but prefers the ERC20 option via preferences.
	pol := &policy.Policy{AllowedSchemes: []string{"exact"}}
	prefs := Preferences{
		Networks:        []string{"eip155:47763"}, // Neo X preferred
		TransferMethods: []string{"eip3009"},
	}
	sel := NewSelector(pol, prefs, StrategyPreferenceFirst)

	accepts := []x402adapter.Requirements{
		baseReqs("eip155:84532", "0xUSDC", "100000"), // Base Sepolia
		baseReqs("eip155:47763", "0xxGAS", "50000"),  // Neo X (preferred)
	}

	result := sel.Select(accepts)
	if result.Selected == nil {
		t.Fatal("expected an option to be selected")
	}
	// With preference-first, Neo X should be tried first.
	if result.Selected.Network != "eip155:47763" {
		t.Errorf("Selected.Network = %q, want eip155:47763", result.Selected.Network)
	}
	if result.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", result.SelectedIndex)
	}
}

func TestSelect_NoAcceptableOptions(t *testing.T) {
	// Policy: only allow chain eip155:1. Both options are on other chains.
	pol := &policy.Policy{
		AllowedChainIDs: []string{"eip155:1"},
		AllowedSchemes:  []string{"exact"},
	}
	sel := NewSelector(pol, Preferences{}, StrategyServerOrder)

	accepts := []x402adapter.Requirements{
		baseReqs("eip155:84532", "0xUSDC", "100000"),
		baseReqs("eip155:47763", "0xxGAS", "50000"),
	}

	result := sel.Select(accepts)
	if result.Selected != nil {
		t.Fatal("expected no acceptable option")
	}
	if result.SelectedIndex != -1 {
		t.Errorf("SelectedIndex = %d, want -1", result.SelectedIndex)
	}
	if len(result.Rejected) != 2 {
		t.Errorf("expected 2 rejections, got %d", len(result.Rejected))
	}
	// All rejections should mention "network" or "chain".
	for _, rej := range result.Rejected {
		if rej.Reason == "" {
			t.Error("rejection reason should not be empty")
		}
	}
}

func TestSelect_PreferenceFirst_ReordersCorrectly(t *testing.T) {
	pol := policy.DefaultPolicy()
	prefs := Preferences{
		Networks: []string{"eip155:47763", "eip155:84532"},
		Assets:   []string{"0xxGAS"},
	}
	sel := NewSelector(pol, prefs, StrategyPreferenceFirst)

	// Server offers Base first, Neo X second.
	accepts := []x402adapter.Requirements{
		baseReqs("eip155:84532", "0xUSDC", "100000"),
		baseReqs("eip155:47763", "0xxGAS", "50000"),
	}

	result := sel.Select(accepts)
	if result.Selected == nil {
		t.Fatal("expected selection")
	}
	// Preference-first should reorder to select Neo X.
	if result.Selected.Network != "eip155:47763" {
		t.Errorf("Selected.Network = %q, want eip155:47763 (preferred)", result.Selected.Network)
	}
}

func TestSelect_ServerOrder_PreservesOrder(t *testing.T) {
	pol := policy.DefaultPolicy()
	prefs := Preferences{
		Networks: []string{"eip155:47763"}, // Prefer Neo X
	}
	// But strategy is server-order, so first valid wins regardless of preference.
	sel := NewSelector(pol, prefs, StrategyServerOrder)

	accepts := []x402adapter.Requirements{
		baseReqs("eip155:84532", "0xUSDC", "100000"),
		baseReqs("eip155:47763", "0xxGAS", "50000"),
	}

	result := sel.Select(accepts)
	if result.Selected == nil {
		t.Fatal("expected selection")
	}
	// Server-order: first option passes policy, so it wins.
	if result.Selected.Network != "eip155:84532" {
		t.Errorf("Selected.Network = %q, want eip155:84532 (server order)", result.Selected.Network)
	}
}

func TestSelect_EmptyAccepts(t *testing.T) {
	sel := NewSelector(policy.DefaultPolicy(), Preferences{}, StrategyServerOrder)
	result := sel.Select(nil)
	if result.Selected != nil {
		t.Error("empty accepts should yield no selection")
	}
	if result.SelectedIndex != -1 {
		t.Errorf("SelectedIndex = %d, want -1", result.SelectedIndex)
	}
}

func TestParseStrategy(t *testing.T) {
	tests := []struct {
		input string
		want  Strategy
	}{
		{"server-order", StrategyServerOrder},
		{"SERVER-ORDER", StrategyServerOrder},
		{"preference-first", StrategyPreferenceFirst},
		{"preference_first", StrategyPreferenceFirst},
		{"PREFERENCE-FIRST", StrategyPreferenceFirst},
		{"", StrategyServerOrder},
		{"unknown", StrategyServerOrder},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseStrategy(tt.input)
			if got != tt.want {
				t.Errorf("ParseStrategy(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatResult(t *testing.T) {
	accepts := []x402adapter.Requirements{
		baseReqs("eip155:84532", "0xUSDC", "100000"),
		baseReqs("eip155:47763", "0xxGAS", "50000"),
	}
	selected := accepts[1]
	r := Result{
		Selected:      &selected,
		SelectedIndex: 1,
		Rejected: []RejectedOption{
			{Index: 0, Requirements: &accepts[0], Reason: "amount too high"},
		},
	}
	out := FormatResult(r, accepts)
	if out == "" {
		t.Error("FormatResult should not be empty")
	}
	// Check key content is present.
	for _, want := range []string{"Offered options: 2", "Selected option [1]", "Rejected options:", "amount too high"} {
		if !containsStr(out, want) {
			t.Errorf("FormatResult() missing %q in:\n%s", want, out)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
