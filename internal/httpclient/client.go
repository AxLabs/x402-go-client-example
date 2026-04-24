// Package httpclient is the buyer-side orchestrator for the x402 payment
// flow. It delegates all protocol concerns (402 parsing, payment payload
// creation, signing, header encoding) to the x402 SDK via
// [x402adapter.Adapter] and limits itself to:
//
//   - issuing the initial HTTP request with stdlib net/http;
//   - detecting HTTP 402 and deciding whether to pay (policy / flags);
//   - retrying the request with SDK-produced payment headers.
//
// This file contains no protocol logic of its own.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bane-labs-org/x402-go-client-example/internal/logging"
	"github.com/bane-labs-org/x402-go-client-example/internal/payment/policy"
	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
)

// Client is a payment-aware HTTP client.
type Client struct {
	httpClient *http.Client
	adapter    *x402adapter.Adapter
	policy     *policy.Policy
	logger     *logging.Logger

	dryRun bool
	noPay  bool
}

// Options configures a Client.
type Options struct {
	Timeout time.Duration
	Adapter *x402adapter.Adapter
	Policy  *policy.Policy
	Logger  *logging.Logger
	DryRun  bool
	NoPay   bool
}

// DefaultOptions returns sensible defaults (no adapter: caller must set one
// if they intend to actually pay).
func DefaultOptions() Options {
	return Options{
		Timeout: 30 * time.Second,
		Policy:  policy.DefaultPolicy(),
	}
}

// New builds a Client. A nil Adapter is allowed for no-pay / dry-run flows.
func New(opts Options) *Client {
	if opts.Policy == nil {
		opts.Policy = policy.DefaultPolicy()
	}
	if opts.Logger == nil {
		opts.Logger = logging.Default()
	}
	return &Client{
		httpClient: &http.Client{Timeout: opts.Timeout},
		adapter:    opts.Adapter,
		policy:     opts.Policy,
		logger:     opts.Logger.WithComponent("httpclient"),
		dryRun:     opts.DryRun,
		noPay:      opts.NoPay,
	}
}

// RequestResult summarises a payment-aware request.
type RequestResult struct {
	Response        *http.Response
	Body            []byte
	PaymentRequired bool
	// Requirements is the SDK-selected payment option that was acted upon.
	Requirements *x402adapter.Requirements
	// PaymentPayload is the signed payload returned by the SDK (nil if no
	// payment was made).
	PaymentPayload *x402adapter.PaymentPayload
	PaymentMade    bool
	Retried        bool
}

// Request is a simple transport-level request description.
type Request struct {
	Method  string
	URL     string
	Body    []byte
	Headers map[string]string
}

// Do performs the x402 payment flow:
//
//  1. Send the request. If not 402, return.
//  2. Ask the SDK to parse the 402.
//  3. Ask the SDK to select acceptable requirements from the offer.
//  4. Short-circuit on no-pay / dry-run.
//  5. Run the local [policy.Policy] against the selected requirements.
//  6. Ask the SDK to build + encode a signed payment header.
//  7. Retry the original request with that header.
func (c *Client) Do(ctx context.Context, req *Request) (*RequestResult, error) {
	c.logger.Debug("Starting request", "method", req.Method, "url", req.URL)

	result, err := c.makeRequest(ctx, req, nil)
	if err != nil {
		return nil, fmt.Errorf("initial request failed: %w", err)
	}

	if result.Response.StatusCode != http.StatusPaymentRequired {
		c.logger.Debug("Request succeeded without payment", "status", result.Response.StatusCode)
		return result, nil
	}

	c.logger.Info("Received 402 Payment Required")
	result.PaymentRequired = true

	if c.adapter == nil {
		c.logger.Warn("No x402 adapter configured; cannot parse 402 response")
		return result, nil
	}

	paymentRequired, err := c.adapter.ParsePaymentRequired(result.Response, result.Body)
	if err != nil {
		return result, fmt.Errorf("failed to parse payment required response: %w", err)
	}

	reqs, err := c.adapter.SelectRequirements(paymentRequired)
	if err != nil {
		return result, fmt.Errorf("no acceptable payment requirements: %w", err)
	}
	result.Requirements = &reqs

	c.logger.Info("Selected payment requirements",
		"scheme", reqs.Scheme,
		"network", reqs.Network,
		"amount", reqs.Amount,
		"asset", reqs.Asset,
		"payTo", reqs.PayTo,
	)

	if c.noPay {
		c.logger.Info("Payment disabled (no-pay)")
		return result, nil
	}

	if err := c.policy.Validate(&reqs); err != nil {
		return result, fmt.Errorf("policy validation failed: %w", err)
	}
	c.logger.Info("Payment requirements passed policy check", "policy", c.policy.String())

	if c.dryRun {
		c.logger.Info("Dry-run mode - not signing or retrying")
		return result, nil
	}

	payload, paymentHeaders, err := c.adapter.CreateAndEncodePayment(ctx, paymentRequired, reqs)
	if err != nil {
		return result, fmt.Errorf("failed to build payment header: %w", err)
	}
	result.PaymentPayload = &payload

	c.logger.Debug("Retrying request with SDK-signed payment header")
	retry, err := c.makeRequest(ctx, req, paymentHeaders)
	if err != nil {
		return result, fmt.Errorf("retry request failed: %w", err)
	}

	retry.PaymentRequired = true
	retry.Requirements = &reqs
	retry.PaymentPayload = &payload
	retry.PaymentMade = true
	retry.Retried = true
	c.logger.Info("Retry completed", "status", retry.Response.StatusCode)
	return retry, nil
}

// makeRequest performs a single HTTP round-trip, optionally adding the SDK's
// payment headers to the request.
func (c *Client) makeRequest(ctx context.Context, req *Request, paymentHeaders map[string]string) (*RequestResult, error) {
	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")
	if len(req.Body) > 0 {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	for k, v := range paymentHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return &RequestResult{Response: resp, Body: body}, nil
}

// Get performs a GET with payment handling.
func (c *Client) Get(ctx context.Context, url string) (*RequestResult, error) {
	return c.Do(ctx, &Request{Method: http.MethodGet, URL: url})
}

// Post performs a POST with payment handling.
func (c *Client) Post(ctx context.Context, url string, body []byte) (*RequestResult, error) {
	return c.Do(ctx, &Request{Method: http.MethodPost, URL: url, Body: body})
}

// SetDryRun toggles dry-run mode.
func (c *Client) SetDryRun(v bool) { c.dryRun = v }

// SetNoPay toggles no-pay mode.
func (c *Client) SetNoPay(v bool) { c.noPay = v }

// FormatRequirements renders SDK-typed requirements for human display.
func FormatRequirements(req *x402adapter.Requirements) string {
	if req == nil {
		return "<no requirements>"
	}
	var b strings.Builder
	b.WriteString("Payment Requirements:\n")
	fmt.Fprintf(&b, "  Scheme:   %s\n", req.Scheme)
	fmt.Fprintf(&b, "  Network:  %s\n", req.Network)
	fmt.Fprintf(&b, "  Amount:   %s\n", req.Amount)
	fmt.Fprintf(&b, "  Asset:    %s\n", req.Asset)
	fmt.Fprintf(&b, "  Pay To:   %s\n", req.PayTo)
	fmt.Fprintf(&b, "  Timeout:  %ds\n", req.MaxTimeoutSeconds)
	return b.String()
}

// FormatResult renders a RequestResult for human display. The signed payment
// payload is printed as JSON rather than by field access so that V1/V2 shape
// differences inside the SDK type do not break the CLI.
func FormatResult(result *RequestResult) string {
	if result == nil {
		return "<no result>"
	}
	var b strings.Builder
	b.WriteString("Request Result:\n")
	fmt.Fprintf(&b, "  Status:           %d\n", result.Response.StatusCode)
	fmt.Fprintf(&b, "  Payment Required: %v\n", result.PaymentRequired)
	fmt.Fprintf(&b, "  Payment Made:     %v\n", result.PaymentMade)
	fmt.Fprintf(&b, "  Retried:          %v\n", result.Retried)

	if result.PaymentPayload != nil {
		if payloadJSON, err := json.MarshalIndent(result.PaymentPayload, "  ", "  "); err == nil {
			fmt.Fprintf(&b, "  Payment Payload:\n  %s\n", payloadJSON)
		}
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, result.Body, "  ", "  "); err == nil {
		fmt.Fprintf(&b, "  Body:\n  %s\n", pretty.String())
	} else {
		fmt.Fprintf(&b, "  Body: %s\n", string(result.Body))
	}
	return b.String()
}
