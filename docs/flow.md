# x402 Payment Flow

This document explains the detailed x402 flow from the client example perspective (used with x402-go-server-example).

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

The server responds with HTTP 402 and payment requirements. The response may
include one or more payment options in the `accepts` array:

```http
HTTP/1.1 402 Payment Required
Content-Type: application/json
PAYMENT-REQUIRED: <base64-encoded payment requirements>

{
  "x402Version": 1,
  "error": "Payment Required",
  "accepts": [
    {
      "scheme": "exact",
      "network": "eip155:84532",
      "maxAmountRequired": "100000",
      "resource": "/paid/hello",
      "description": "Pay with USDC on Base Sepolia",
      "payTo": "0x1234567890123456789012345678901234567890",
      "asset": "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
      "maxTimeoutSeconds": 300
    },
    {
      "scheme": "exact",
      "network": "eip155:47763",
      "maxAmountRequired": "50000",
      "resource": "/paid/hello",
      "description": "Pay with xGAS on Neo X",
      "payTo": "0x2222222222222222222222222222222222222222",
      "asset": "0xABCDEF0000000000000000000000000000000001",
      "maxTimeoutSeconds": 300
    }
  ]
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
1. The `PAYMENT-REQUIRED` header (base64-encoded, preferred in v2)
2. The response body's `accepts` array (v1 form)

The SDK handles both formats transparently.

### Step 3b: Multi-Option Selection

When the server offers multiple payment options, the client evaluates **all** of
them before committing to one:

1. **Enumerate** all `accepts` entries.
2. **Order** candidates based on the selection strategy:
   - `server-order`: preserve server-provided ordering (default, backward compatible).
   - `preference-first`: reorder by preference score (network > asset > transfer method).
3. **Validate** each candidate against local policy (amount, chain, asset, payTo, scheme).
4. **Select** the first candidate that passes all checks.
5. **Report** rejection reasons for all rejected candidates (diagnostics).

If **no** option passes policy, the client returns a descriptive error listing
each rejected option and its failure reason.

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

The client retries the original request with the `PAYMENT-SIGNATURE` header:

```http
GET /paid/hello HTTP/1.1
Host: api.example.com
Accept: application/json
PAYMENT-SIGNATURE: <base64-encoded signed payment payload>
```

(Legacy implementations may use `X-PAYMENT` — the SDK handles both.)

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
      │     + accepts: [{option A}, {option B}, ...]    │
      │<────────────────────────────────────────────────│
      │                                                 │
      │  3. Parse all offered options                   │
      │                                                 │
      │  3b. Multi-option selection                     │
      │     ┌─────────────────────────┐                 │
      │     │ For each option:        │                 │
      │     │ - Reorder by preference │                 │
      │     │ - Check policy          │                 │
      │     │ - Select first passing  │                 │
      │     │ - Log rejections        │                 │
      │     └─────────────────────────┘                 │
      │                                                 │
      │  4. Validate selected option against policy     │
      │     ┌─────────────────────┐                     │
      │     │ Policy Check        │                     │
      │     │ - Amount OK?        │                     │
      │     │ - Chain OK?         │                     │
      │     │ - Recipient OK?     │                     │
      │     └─────────────────────┘                     │
      │                                                 │
      │  5. Sign authorization (SDK)                    │
      │     ┌─────────────────────┐                     │
      │     │ Create Authorization│                     │
      │     │ Sign with EIP-712   │                     │
      │     └─────────────────────┘                     │
      │                                                 │
      │  6. GET /paid/hello                             │
      │     PAYMENT-SIGNATURE: {signed payload}         │
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
