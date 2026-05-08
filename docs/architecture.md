# Architecture

This document describes the architecture and design decisions of the x402-go-client-example (the client-side example paired with x402-go-server-example).

## Overview

The client is structured as a modular Go application with clear separation of concerns:

```text
┌─────────────────────────────────────────────────────────────────────┐
│                           CLI Layer                                  │
│                       (cmd/client, cli/)                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │
│  │   config/   │  │  logging/   │  │  version/   │                 │
│  │             │  │             │  │             │                 │
│  │ Environment │  │  Structured │  │   Build     │                 │
│  │ Variables   │  │  Logging    │  │   Info      │                 │
│  └─────────────┘  └─────────────┘  └─────────────┘                 │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│                         HTTP Client Layer                           │
│                        (httpclient/)                                │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    Payment-Aware Client                      │   │
│  │                                                              │   │
│  │  - Initial request (no payment)                              │   │
│  │  - Detect 402 response                                       │   │
│  │  - Parse requirements                                        │   │
│  │  - Policy validation                                         │   │
│  │  - Sign and retry                                            │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│                         Payment Layer                               │
│                                                                     │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐    │
│  │  payment/model  │  │ payment/policy  │  │     signer      │    │
│  │                 │  │                 │  │                 │    │
│  │ - Requirements  │  │ - Max amount    │  │ - Ethereum      │    │
│  │ - Authorization │  │ - Allowed assets│  │   signing       │    │
│  │ - PaymentHeader │  │ - Allowed chains│  │ - Mock signer   │    │
│  │ - Response402   │  │ - Allowed payTo │  │   for tests     │    │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Package Structure

### `cmd/client/`

Entry point for the CLI application. Minimal code - just initializes and runs the CLI app.

### `internal/cli/`

Implements the Cobra CLI commands:
- `get` - GET request with payment handling
- `post` - POST request with payment handling
- `inspect` - Fetch 402 requirements without paying
- `version` - Print version information

Responsibilities:
- Parse command-line flags
- Set up dependencies (client, signer, policy)
- Execute requests and display results
- Human-friendly console output

### `internal/config/`

Configuration loading from environment variables.

Features:
- Default values for all settings
- Environment variable parsing
- Validation
- Type conversion helpers

### `internal/httpclient/`

Payment-aware HTTP client that implements the x402 flow.

Key design decisions:
1. **Single responsibility**: The client handles the entire 402 flow internally
2. **Composable**: Accepts signer, policy, and logger as dependencies
3. **Testable**: All dependencies can be mocked
4. **Observable**: Returns detailed results about the flow
5. **Domain-correct signing**: For EIP-3009, the client signs against the
	token-specific EIP-712 domain and supports explicit `name`/`version`
	overrides when server metadata is incorrect; Permit2 uses its own contract
	domain model

### `internal/payment/model/`

Data structures for the x402 protocol:
- `PaymentRequirements` - What the server demands
- `Authorization` - What the client authorizes
- `PaymentHeader` - The signed header sent on retry
- `Response402` - Parsed 402 response body

### `internal/payment/policy/`

Client-side payment policy enforcement.

Policy checks:
- Maximum payment amount
- Allowed payment schemes
- Allowed blockchain networks/chains
- Allowed payment recipient addresses

Design:
- Returns detailed violation information
- Multiple violations can be returned at once
- Easy to extend with new policy types

### `internal/signer/`

Abstraction for signing payment authorizations.

Implementations:
- `EthereumSigner` - Real signing with secp256k1
- `MockSigner` - For testing without real keys

### `internal/logging/`

Structured logging using Go's `slog` package.

Features:
- Configurable log levels
- JSON or text output
- Component tagging

### `internal/version/`

Build-time version information set via ldflags.

## Key Design Decisions

### 1. Explicit Payment Flow

The client makes the payment flow explicit:
1. Initial request without payment
2. Parse 402 response
3. Validate against policy
4. Sign authorization
5. Retry with payment header

This is intentional - the caller must be aware they are paying.

### 2. Policy-First Security

Payment policy is checked BEFORE signing. The client never signs a payment that violates local policy.

### 3. Separation of CLI and Core Logic

The `httpclient` package contains all payment flow logic. The `cli` package only handles user interaction. This allows:
- Using the client as a library
- Easy testing of payment logic
- Different UIs (CLI, GUI, etc.)

### 4. Dependency Injection

All dependencies are injected:
- Signer can be real or mock
- Policy is configurable
- Logger is provided externally

This makes testing straightforward and allows flexible configuration.

### 5. No Global State

No global variables or singletons. All state is passed explicitly.

## Error Handling

Errors are categorized:
- **Configuration errors** - Invalid config, missing required values
- **Network errors** - Target unreachable, timeout
- **Protocol errors** - Malformed 402, invalid requirements
- **Policy errors** - Requirements violate local policy
- **Signing errors** - Failed to sign authorization
- **Payment errors** - Retry failed, server rejected payment

Each error type is handled appropriately with clear messages.

## Testing Strategy

1. **Unit tests** - Each package has `_test.go` files
2. **Integration tests** - `test/integration_test.go` tests the full flow
3. **Mock server** - Uses `httptest` to simulate the paid server

## Security Considerations

1. **Private keys** - Never logged, not serialized in config output
2. **Policy enforcement** - Always validated before signing
3. **Amount validation** - Prevents signing excessive payments
4. **Address validation** - Can restrict to known recipients
