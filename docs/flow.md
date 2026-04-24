# x402 Payment Flow

This document explains the detailed flow of the x402 payment protocol from the buyer/client perspective.

## Protocol Overview

The x402 protocol extends HTTP with payment capabilities. When a client requests a protected resource, the server responds with `402 Payment Required` and includes payment requirements. The client can then authorize payment and retry the request.

## Detailed Flow

### Step 1: Initial Request

The client makes a standard HTTP request to the protected endpoint:

```http
GET /paid/hello HTTP/1.1
Host: api.example.com
Accept: application/json
```

At this point, no payment headers are included.

### Step 2: Server Returns 402

The server responds with HTTP 402 and payment requirements:

```http
HTTP/1.1 402 Payment Required
Content-Type: application/json
X-Payment-Requirements: {...}

{
  "error": "Payment Required",
  "paymentRequirements": {
    "scheme": "exact",
    "network": "84532",
    "maxAmountRequired": "100000",
    "resource": "/paid/hello",
    "description": "Access to hello endpoint",
    "payTo": "0x1234567890123456789012345678901234567890"
  }
}
```

#### Payment Requirements Fields

| Field | Description |
|-------|-------------|
| `scheme` | Payment scheme (e.g., "exact") |
| `network` | Blockchain network/chain ID |
| `maxAmountRequired` | Maximum amount to be charged (smallest unit) |
| `resource` | The protected resource path |
| `description` | Human-readable description |
| `payTo` | Recipient address for payment |

### Step 3: Client Parses Requirements

The client extracts payment requirements from either:
1. The `X-Payment-Requirements` header (preferred)
2. The response body's `paymentRequirements` field

```go
// From header
req, err := model.ParsePaymentRequirementsFromHeader(headerValue)

// From body
resp, err := model.Parse402Response(body)
req := resp.PaymentRequirements
```

### Step 4: Policy Validation

**Critical step**: Before signing anything, the client validates requirements against local policy:

```go
policy := &Policy{
    MaxAmount:       1000000,        // Max 1 USDC
    AllowedChainIDs: []string{"84532"}, // Only Base Sepolia
    AllowedPayTo:    []string{...},  // Known recipients
}

err := policy.Validate(requirements)
if err != nil {
    // STOP - do not sign
    return PolicyViolationError(err)
}
```

Policy checks include:
- **Amount**: Is `maxAmountRequired` within acceptable limits?
- **Chain**: Is the `network` an allowed blockchain?
- **Recipient**: Is `payTo` a trusted address?
- **Scheme**: Is the payment scheme supported?

If ANY policy check fails, the client **MUST NOT** sign the payment.

### Step 5: Sign Payment Authorization

If policy passes, the client constructs and signs the authorization:

```go
authorization := &Authorization{
    From:        signerAddress,        // Client's address
    To:          requirements.PayTo,   // Server's address
    Value:       requirements.MaxAmountRequired,
    ValidAfter:  time.Now().Add(-1*time.Minute).Unix(),
    ValidBefore: time.Now().Add(5*time.Minute).Unix(),
    Nonce:       generateNonce(),
}

// Create signature message
message := createSignatureMessage(requirements, authorization)

// Sign with Ethereum personal_sign
signature := personalSign(message, privateKey)
```

The signature authorizes the exact payment described in the requirements.

### Step 6: Retry with Payment Header

The client retries the original request with the `X-PAYMENT` header:

```http
GET /paid/hello HTTP/1.1
Host: api.example.com
Accept: application/json
X-PAYMENT: {"scheme":"exact","network":"84532","payload":{"signature":"0x...","authorization":{...}}}
```

#### Payment Header Structure

```json
{
  "scheme": "exact",
  "network": "84532",
  "payload": {
    "signature": "0x...",
    "authorization": {
      "from": "0xClientAddress",
      "to": "0xServerAddress",
      "value": "100000",
      "validAfter": 1703001000,
      "validBefore": 1703001300,
      "nonce": "123456789"
    }
  }
}
```

### Step 7: Server Verifies and Responds

The server:
1. Extracts the `X-PAYMENT` header
2. Verifies the signature
3. Checks authorization validity (time bounds, nonce)
4. Settles the payment (on-chain or off-chain)
5. Returns the protected content

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "message": "Hello, authorized payer!",
  "status": "success"
}
```

## Flow Diagram

```
┌────────────┐                                    ┌────────────┐
│   Client   │                                    │   Server   │
└─────┬──────┘                                    └─────┬──────┘
      │                                                 │
      │  1. GET /paid/hello                             │
      │────────────────────────────────────────────────>│
      │                                                 │
      │  2. 402 Payment Required                        │
      │     + PaymentRequirements                       │
      │<────────────────────────────────────────────────│
      │                                                 │
      │  3. Parse requirements                          │
      │                                                 │
      │  4. Validate against policy                     │
      │     ┌─────────────────────┐                     │
      │     │ Policy Check        │                     │
      │     │ - Amount OK?        │                     │
      │     │ - Chain OK?         │                     │
      │     │ - Recipient OK?     │                     │
      │     └─────────────────────┘                     │
      │                                                 │
      │  5. Sign authorization                          │
      │     ┌─────────────────────┐                     │
      │     │ Create Authorization│                     │
      │     │ Sign with ECDSA     │                     │
      │     └─────────────────────┘                     │
      │                                                 │
      │  6. GET /paid/hello                             │
      │     X-PAYMENT: {signed authorization}           │
      │────────────────────────────────────────────────>│
      │                                                 │
      │                                   7. Verify sig │
      │                                      Settle pay │
      │                                                 │
      │  8. 200 OK                                      │
      │     Protected content                           │
      │<────────────────────────────────────────────────│
      │                                                 │
```

## Error Scenarios

### Policy Violation

If requirements violate policy, the flow stops at step 4:

```
Client: GET /paid/expensive
Server: 402 (amount: 50000000)
Client: Policy check FAILED (max: 1000000)
Client: EXIT with error - no payment signed
```

### Signing Failure

If signing fails (e.g., no private key):

```
Client: GET /paid/hello
Server: 402
Client: Policy check passed
Client: SIGN FAILED - no private key
Client: EXIT with error
```

### Payment Rejected

If the server rejects the payment header:

```
Client: GET /paid/hello
Server: 402
Client: Policy check passed
Client: Signed payment
Client: RETRY with X-PAYMENT header
Server: 400 Bad Request (invalid signature)
Client: EXIT with error
```

## Dry-Run Mode

In dry-run mode, the flow stops after policy validation:

```
Client: GET /paid/hello --dry-run
Server: 402
Client: Parse requirements
Client: Display requirements
Client: Policy check (passed/failed)
Client: STOP - dry-run mode, no signing
```

This is useful for previewing what a payment would require.

## No-Pay Mode

In no-pay mode, the flow stops immediately after receiving 402:

```
Client: GET /paid/hello --no-pay
Server: 402
Client: Parse requirements
Client: Display requirements
Client: STOP - no-pay mode
```

## Security Properties

1. **No automatic payment**: Client always checks policy before signing
2. **Time-limited authorization**: ValidAfter/ValidBefore window
3. **Unique authorization**: Nonce prevents replay
4. **Specific recipient**: Payment only valid for specified address
5. **Amount ceiling**: MaxAmountRequired is maximum, not exact
