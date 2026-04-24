// Package policy implements client-side payment policy validation.
//
// Policy is the single place where the buyer decides whether the payment
// requirements presented by a server are acceptable. It operates on the SDK
// type [x402adapter.Requirements] (= x402 v2 PaymentRequirements) and does
// not know anything about wire formats, signing, or the SDK internals.
package policy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bane-labs-org/x402-buyer-client-go/internal/x402adapter"
)

// Policy defines the constraints for accepting payment requirements.
type Policy struct {
	// MaxAmount is the maximum payment amount allowed (in smallest unit).
	// A value of 0 means no limit.
	MaxAmount uint64

	// AllowedAssets is the list of allowed asset identifiers (e.g. token
	// contract addresses). Comparison is case-insensitive. Empty = any.
	AllowedAssets []string

	// AllowedChainIDs is the list of allowed network identifiers. The SDK
	// uses CAIP-2 format (e.g. "eip155:84532"). Empty = any.
	AllowedChainIDs []string

	// AllowedPayTo is the list of allowed recipient addresses. Empty = any.
	AllowedPayTo []string

	// AllowedSchemes is the list of allowed payment schemes.
	// If empty, defaults to ["exact"].
	AllowedSchemes []string
}

// DefaultPolicy returns a permissive policy suitable for local development.
func DefaultPolicy() *Policy {
	return &Policy{
		AllowedSchemes: []string{"exact"},
	}
}

// NewPolicyFromConfig is a convenience constructor used by the CLI.
func NewPolicyFromConfig(maxAmount uint64, allowedAssets, allowedChainIDs, allowedPayTo []string) *Policy {
	return &Policy{
		MaxAmount:       maxAmount,
		AllowedAssets:   allowedAssets,
		AllowedChainIDs: allowedChainIDs,
		AllowedPayTo:    allowedPayTo,
		AllowedSchemes:  []string{"exact"},
	}
}

// PolicyViolation describes a single rejected field.
type PolicyViolation struct {
	Field         string
	Message       string
	RequiredValue string
	PolicyValue   string
}

// Error implements the error interface.
func (v PolicyViolation) Error() string {
	return fmt.Sprintf("policy violation [%s]: %s (required: %s, allowed: %s)",
		v.Field, v.Message, v.RequiredValue, v.PolicyValue)
}

// PolicyViolations aggregates multiple violations.
type PolicyViolations []PolicyViolation

// Error implements the error interface.
func (vs PolicyViolations) Error() string {
	if len(vs) == 0 {
		return "no violations"
	}
	if len(vs) == 1 {
		return vs[0].Error()
	}
	msgs := make([]string, 0, len(vs))
	for _, v := range vs {
		msgs = append(msgs, v.Error())
	}
	return fmt.Sprintf("%d policy violations: %s", len(vs), strings.Join(msgs, "; "))
}

// Validate checks that the given SDK-typed requirements pass every rule in
// this policy. Returns nil, a single PolicyViolation, or PolicyViolations.
func (p *Policy) Validate(req *x402adapter.Requirements) error {
	if req == nil {
		return errors.New("payment requirements cannot be nil")
	}

	var violations PolicyViolations
	if v := p.validateScheme(req.Scheme); v != nil {
		violations = append(violations, *v)
	}
	if v := p.validateAmount(req.Amount); v != nil {
		violations = append(violations, *v)
	}
	if v := p.validateChain(req.Network); v != nil {
		violations = append(violations, *v)
	}
	if v := p.validatePayTo(req.PayTo); v != nil {
		violations = append(violations, *v)
	}
	if v := p.validateAsset(req.Asset); v != nil {
		violations = append(violations, *v)
	}

	if len(violations) > 0 {
		return violations
	}
	return nil
}

func (p *Policy) validateScheme(scheme string) *PolicyViolation {
	allowed := p.AllowedSchemes
	if len(allowed) == 0 {
		allowed = []string{"exact"}
	}
	for _, a := range allowed {
		if strings.EqualFold(scheme, a) {
			return nil
		}
	}
	return &PolicyViolation{
		Field: "scheme", Message: "payment scheme not allowed",
		RequiredValue: scheme, PolicyValue: strings.Join(allowed, ", "),
	}
}

func (p *Policy) validateAmount(amountStr string) *PolicyViolation {
	if p.MaxAmount == 0 {
		return nil
	}
	amount, err := strconv.ParseUint(amountStr, 10, 64)
	if err != nil {
		return &PolicyViolation{
			Field: "amount", Message: "invalid amount format",
			RequiredValue: amountStr, PolicyValue: "valid uint64",
		}
	}
	if amount > p.MaxAmount {
		return &PolicyViolation{
			Field: "amount", Message: "amount exceeds maximum allowed",
			RequiredValue: amountStr, PolicyValue: strconv.FormatUint(p.MaxAmount, 10),
		}
	}
	return nil
}

func (p *Policy) validateChain(network string) *PolicyViolation {
	if len(p.AllowedChainIDs) == 0 {
		return nil
	}
	for _, a := range p.AllowedChainIDs {
		if network == a {
			return nil
		}
	}
	return &PolicyViolation{
		Field: "network", Message: "chain/network not allowed",
		RequiredValue: network, PolicyValue: strings.Join(p.AllowedChainIDs, ", "),
	}
}

func (p *Policy) validatePayTo(payTo string) *PolicyViolation {
	if len(p.AllowedPayTo) == 0 {
		return nil
	}
	for _, a := range p.AllowedPayTo {
		if strings.EqualFold(payTo, a) {
			return nil
		}
	}
	return &PolicyViolation{
		Field: "payTo", Message: "payment recipient not allowed",
		RequiredValue: payTo, PolicyValue: strings.Join(p.AllowedPayTo, ", "),
	}
}

func (p *Policy) validateAsset(asset string) *PolicyViolation {
	if len(p.AllowedAssets) == 0 || asset == "" {
		return nil
	}
	for _, a := range p.AllowedAssets {
		if strings.EqualFold(asset, a) {
			return nil
		}
	}
	return &PolicyViolation{
		Field: "asset", Message: "asset not allowed",
		RequiredValue: asset, PolicyValue: strings.Join(p.AllowedAssets, ", "),
	}
}

// String returns a human-readable description for logging.
func (p *Policy) String() string {
	var parts []string
	if p.MaxAmount > 0 {
		parts = append(parts, fmt.Sprintf("maxAmount=%d", p.MaxAmount))
	} else {
		parts = append(parts, "maxAmount=unlimited")
	}
	if len(p.AllowedAssets) > 0 {
		parts = append(parts, fmt.Sprintf("assets=[%s]", strings.Join(p.AllowedAssets, ",")))
	}
	if len(p.AllowedChainIDs) > 0 {
		parts = append(parts, fmt.Sprintf("chains=[%s]", strings.Join(p.AllowedChainIDs, ",")))
	}
	if len(p.AllowedPayTo) > 0 {
		parts = append(parts, fmt.Sprintf("payTo=[%d addresses]", len(p.AllowedPayTo)))
	}
	return fmt.Sprintf("Policy{%s}", strings.Join(parts, ", "))
}
