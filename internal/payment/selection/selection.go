// Package selection implements multi-option payment requirement selection.
//
// When a server returns multiple payment options in its 402 response, this
// package evaluates each candidate against local policy and user preferences,
// then picks the best acceptable option according to the configured strategy.
package selection

import (
	"fmt"
	"strings"

	"github.com/bane-labs-org/x402-go-client-example/internal/payment/policy"
	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
)

// Strategy defines how candidates are ordered for selection.
type Strategy string

const (
	// StrategyServerOrder evaluates candidates in the order the server provided,
	// selecting the first that passes policy. This preserves backward compatibility.
	StrategyServerOrder Strategy = "server-order"

	// StrategyPreferenceFirst reorders candidates based on local preferences
	// before applying policy checks. Higher-preference candidates are tried first.
	StrategyPreferenceFirst Strategy = "preference-first"
)

// ParseStrategy converts a string to a Strategy, defaulting to StrategyServerOrder.
func ParseStrategy(s string) Strategy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "preference-first", "preference_first":
		return StrategyPreferenceFirst
	default:
		return StrategyServerOrder
	}
}

// Preferences captures the user's preferred networks, assets, and transfer methods.
type Preferences struct {
	Networks        []string // CAIP-2 preferred networks (highest priority first)
	Assets          []string // Preferred asset addresses (highest priority first)
	TransferMethods []string // Preferred transfer methods: "eip3009", "permit2"
}

// RejectedOption records why a specific candidate was not selected.
type RejectedOption struct {
	Index        int
	Requirements *x402adapter.Requirements
	Reason       string
}

// Result holds the outcome of a selection attempt.
type Result struct {
	// Selected is the chosen option, or nil if no acceptable option was found.
	Selected *x402adapter.Requirements
	// SelectedIndex is the index in the original accepts list (-1 if none).
	SelectedIndex int
	// Rejected lists each rejected option with its reason.
	Rejected []RejectedOption
}

// Selector evaluates multiple payment options and picks the best acceptable one.
type Selector struct {
	policy      *policy.Policy
	preferences Preferences
	strategy    Strategy
}

// NewSelector builds a selector with the given policy, preferences, and strategy.
func NewSelector(pol *policy.Policy, prefs Preferences, strat Strategy) *Selector {
	if pol == nil {
		pol = policy.DefaultPolicy()
	}
	return &Selector{
		policy:      pol,
		preferences: prefs,
		strategy:    strat,
	}
}

// Select evaluates all candidates and returns the best acceptable option.
// It returns a Result that includes the selected option (if any) and
// rejection reasons for all other candidates.
func (s *Selector) Select(accepts []x402adapter.Requirements) Result {
	if len(accepts) == 0 {
		return Result{SelectedIndex: -1}
	}

	// Build indexed candidates so we can reorder without losing original position.
	type indexed struct {
		idx int
		req x402adapter.Requirements
	}
	candidates := make([]indexed, len(accepts))
	for i := range accepts {
		candidates[i] = indexed{idx: i, req: accepts[i]}
	}

	// If preference-first, sort candidates by preference score (stable, descending).
	if s.strategy == StrategyPreferenceFirst {
		stableSortByScore(candidates, func(c indexed) int {
			return s.preferenceScore(&c.req)
		})
	}

	result := Result{SelectedIndex: -1}

	for _, c := range candidates {
		req := c.req
		err := s.policy.Validate(&req)
		if err != nil {
			result.Rejected = append(result.Rejected, RejectedOption{
				Index:        c.idx,
				Requirements: &req,
				Reason:       err.Error(),
			})
			continue
		}
		// First candidate that passes policy wins.
		result.Selected = &req
		result.SelectedIndex = c.idx
		// Record remaining candidates as "not evaluated (already selected)".
		break
	}

	return result
}

// preferenceScore computes a score for a candidate based on user preferences.
// Higher score = stronger preference. Score is the sum of:
//   - network match bonus (highest priority)
//   - asset match bonus
//   - transfer method match bonus
func (s *Selector) preferenceScore(req *x402adapter.Requirements) int {
	score := 0

	// Network preference (higher position = higher bonus).
	for i, n := range s.preferences.Networks {
		if strings.EqualFold(req.Network, n) {
			score += (len(s.preferences.Networks) - i) * 1000
			break
		}
	}

	// Asset preference.
	for i, a := range s.preferences.Assets {
		if strings.EqualFold(req.Asset, a) {
			score += (len(s.preferences.Assets) - i) * 100
			break
		}
	}

	// Transfer method preference (matched via Extra field or scheme hints).
	method := inferTransferMethod(req)
	for i, m := range s.preferences.TransferMethods {
		if strings.EqualFold(method, m) {
			score += (len(s.preferences.TransferMethods) - i) * 10
			break
		}
	}

	return score
}

// inferTransferMethod reads assetTransferMethod from requirements.Extra when set.
// For scheme "exact" without extra, the protocol default is eip3009.
func inferTransferMethod(req *x402adapter.Requirements) string {
	if req == nil {
		return ""
	}
	if req.Extra != nil {
		if raw, ok := req.Extra["assetTransferMethod"]; ok {
			if method, ok := raw.(string); ok && method != "" {
				return strings.ToLower(method)
			}
		}
	}
	if strings.EqualFold(req.Scheme, "exact") {
		return "eip3009"
	}
	return strings.ToLower(req.Scheme)
}

// stableSortByScore sorts candidates by descending score, preserving original
// order for equal scores (stable).
func stableSortByScore[T any](items []T, score func(T) int) {
	// Simple insertion sort (stable, fine for small N which is typical for accepts lists).
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && score(items[j]) > score(items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// FormatResult produces a human-readable summary of the selection result.
func FormatResult(r Result, allAccepts []x402adapter.Requirements) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Offered options: %d\n", len(allAccepts)))
	for i, a := range allAccepts {
		b.WriteString(fmt.Sprintf("  [%d] scheme=%s network=%s asset=%s amount=%s payTo=%s\n",
			i, a.Scheme, a.Network, a.Asset, a.Amount, a.PayTo))
	}

	if r.Selected != nil {
		b.WriteString(fmt.Sprintf("\nSelected option [%d]: scheme=%s network=%s asset=%s amount=%s\n",
			r.SelectedIndex, r.Selected.Scheme, r.Selected.Network, r.Selected.Asset, r.Selected.Amount))
	} else {
		b.WriteString("\nNo acceptable option found.\n")
	}

	if len(r.Rejected) > 0 {
		b.WriteString("\nRejected options:\n")
		for _, rej := range r.Rejected {
			b.WriteString(fmt.Sprintf("  [%d] %s — reason: %s\n",
				rej.Index, rej.Requirements.Network, rej.Reason))
		}
	}

	return b.String()
}
