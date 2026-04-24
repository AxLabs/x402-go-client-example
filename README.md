# x402-buyer-client-go

A production-style Go client that demonstrates the **buyer/caller side** of the x402 payment protocol flow.

## Overview

This client application shows how to:

- Call a protected endpoint
- Handle `402 Payment Required` responses
- Parse and inspect payment requirements
- Validate requirements against local payment policy
- Sign payment authorization
- Retry the request with payment headers
- Display the final protected response

> **Important**: The caller is responsible for paying and retrying. This is NOT a mock flow where payment happens automatically or where the server pays on behalf of the client.

## How the x402 Buyer Flow Works

```text
┌─────────────────────────────────────────────────────────────────────┐
│                         x402 BUYER FLOW                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. Client sends request to protected endpoint                      │
│                              │                                      │
│                              ▼                                      │
│  2. Server responds with 402 Payment Required                       │
│     + Payment requirements (amount, address, network, etc.)         │
│                              │                                      │
│                              ▼                                      │
│  3. Client parses payment requirements                              │
│                              │                                      │
│                              ▼                                      │
│  4. Client validates requirements against LOCAL POLICY              │
│     (max amount, allowed chains, allowed recipients)                │
│                              │                                      │
│                     ┌────────┴────────┐                             │
│                     │                 │                             │
│              Policy PASS        Policy FAIL                         │
│                     │                 │                             │
│                     ▼                 ▼                             │
│  5. Client SIGNS payment     Client exits with error                │
│     authorization                                                   │
│                     │                                               │
│                     ▼                                               │
│  6. Client RETRIES request with X-PAYMENT header                    │
│                     │                                               │
│                     ▼                                               │
│  7. Server verifies payment and returns protected content           │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Prerequisites

- Go 1.22 or later
- Access to a paid-server endpoint (companion repo)
- An Ethereum private key for signing (for actual payments)

## Installation

```bash
# Clone the repository
git clone https://github.com/bane-labs-org/x402-buyer-client-go.git
cd x402-buyer-client-go

# Install dependencies
go mod download

# Build the client
make build
```

## Configuration

### Environment Variables

| Variable | Description | Default |
| -------- | ----------- | ------- |
| `CLIENT_LOG_LEVEL` | Logging level (debug, info, warn, error) | `info` |
| `CLIENT_LOG_JSON` | Enable JSON logging format | `false` |
| `CLIENT_PRIVATE_KEY` | Ethereum private key for signing (hex, with or without 0x prefix) | - |
| `CLIENT_MAX_AMOUNT` | Maximum payment amount allowed (smallest unit, e.g., wei) | `1000000` |
| `CLIENT_ALLOWED_ASSET` | Comma-separated list of allowed assets | - |
| `CLIENT_ALLOWED_CHAIN_ID` | Allowed blockchain chain/network ID | - |
| `CLIENT_ALLOWED_PAY_TO` | Comma-separated list of allowed payment recipient addresses | - |
| `CLIENT_TIMEOUT` | HTTP request timeout | `30s` |
| `CLIENT_DRY_RUN` | Parse 402 but don't sign or retry | `false` |
| `CLIENT_NO_PAY` | Don't attempt payment flow at all | `false` |

### CLI Flags

Many settings can also be controlled via CLI flags:

```bash
x402-client get --url <URL> [--dry-run] [--no-pay] [--verbose] [--timeout <duration>]
```

### Example .env File

```bash
# Copy .env.example to .env and customize
cp .env.example .env
```

## Usage

### Basic GET Request

```bash
# Make a GET request to a protected endpoint
go run ./cmd/client get --url http://localhost:8080/paid/hello
```

### POST Request with Body

```bash
# Make a POST request with JSON body
go run ./cmd/client post --url http://localhost:8080/paid/echo --body '{"message":"hello"}'
```

### Inspect Payment Requirements (No Payment)

```bash
# Fetch and display 402 requirements without paying
go run ./cmd/client inspect --url http://localhost:8080/paid/hello
```

### Dry-Run Mode

```bash
# Parse requirements and validate policy, but don't sign or retry
go run ./cmd/client get --url http://localhost:8080/paid/hello --dry-run
```

### Version

```bash
go run ./cmd/client version
```

## Example Output

### Successful Payment Flow

```text
==> Starting x402 payment flow
    Method: GET
    URL: http://localhost:8080/paid/hello
    Signer Address: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
    Policy: Policy{maxAmount=1000000, chains=[84532]}

==> Making initial request

==> Received 402 Payment Required
Payment Requirements:
  Scheme:      exact
  Network:     84532
  Amount:      100000
  Pay To:      0x1234567890123456789012345678901234567890
  Resource:    /paid/hello

==> Payment authorized and retry completed
    From: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
    To: 0x1234567890123456789012345678901234567890
    Amount: 100000

==> Final Response
    Status: 200 OK
    Content-Type: application/json

Body:
{
  "message": "Hello, authorized payer!",
  "status": "success"
}
```

### Policy Rejection (Amount Exceeds Maximum)

```text
==> Starting x402 payment flow
    Method: GET
    URL: http://localhost:8080/paid/expensive
    Signer Address: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
    Policy: Policy{maxAmount=1000000}

==> Making initial request

==> Received 402 Payment Required
Payment Requirements:
  Scheme:      exact
  Network:     84532
  Amount:      50000000
  Pay To:      0x1234567890123456789012345678901234567890
  Resource:    /paid/expensive

[ERROR] Request failed: policy validation failed: policy violation [amount]: amount exceeds maximum allowed (required: 50000000, allowed: 1000000)
```

## Client Payment Policy

The client enforces local payment policies before authorizing any payment:

### Maximum Amount

```bash
export CLIENT_MAX_AMOUNT=1000000  # Max 1 USDC (6 decimals)
```

### Allowed Chains

```bash
export CLIENT_ALLOWED_CHAIN_ID=84532  # Only Base Sepolia
```

### Allowed Recipients

```bash
export CLIENT_ALLOWED_PAY_TO=0x1234...,0x5678...  # Only these addresses
```

If the 402 response requires payment that violates any policy:

- The client will NOT sign the payment
- The client will exit with a clear error message
- No funds will be authorized or transferred

## Companion Repository

This client is designed to work with the **paid-server** companion repository, which implements the server side of the x402 protocol. The server:

- Exposes protected endpoints
- Returns 402 Payment Required for unauthorized requests
- Verifies payment headers
- Returns protected content after successful payment verification

## Development

### Running Tests

```bash
make test
```

### Building

```bash
make build
```

### Linting

```bash
make lint
```

## Project Structure

```text
x402-buyer-client-go/
├── cmd/
│   └── client/
│       └── main.go              # CLI entry point
├── internal/
│   ├── cli/                     # CLI command implementations
│   ├── config/                  # Configuration loading
│   ├── httpclient/              # HTTP client with 402 handling
│   ├── logging/                 # Structured logging
│   ├── payment/
│   │   ├── model/               # Payment data structures
│   │   └── policy/              # Payment policy validation
│   ├── signer/                  # Payment signing abstraction
│   └── version/                 # Version information
├── test/                        # Integration tests
├── docs/                        # Documentation
├── .env.example
├── Makefile
└── README.md
```

## Documentation

- [Architecture](docs/architecture.md) - System architecture and design decisions
- [Flow](docs/flow.md) - Detailed x402 payment flow explanation

## License

MIT License
