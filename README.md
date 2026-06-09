# x402-go-client-example

A Go **example client** for the buyer/caller side of the x402 payment flow.

This repository is intentionally paired with **`x402-go-server-example`**.
Use both repos together to run a complete local example of `402 Payment Required` -> payment authorization -> retry.

## What This Example Shows

- Calling a protected endpoint
- Handling `402 Payment Required` (single and multi-option responses)
- Parsing payment requirements
- Evaluating multiple payment options with configurable selection strategy
- Applying local payment policy checks per candidate
- Creating payment headers via the official x402 Go SDK
- Retrying the request with payment headers

## Paired Repositories

- Client (this repo): `x402-go-client-example`
- Server (counterpart): `x402-go-server-example`

## Quick Start (End-to-End Example)

1. Start the server example.

```bash
# In a separate terminal, clone and run the server example.
# Follow x402-go-server-example README for its exact startup command.
```

2. Clone and build this client example.

```bash
git clone https://github.com/AxLabs/x402-go-client-example.git
cd x402-go-client-example
make build
```

3. Configure the client.

```bash
cp .env.example .env
```

Set these in `.env`:

- `CLIENT_PRIVATE_KEY` for real signing flows
- Optional safety controls like `CLIENT_MAX_AMOUNT`, `CLIENT_ALLOWED_CHAIN_ID`, `CLIENT_ALLOWED_PAY_TO`

Load `.env` into your current shell (required):

```bash
set -a
source .env
set +a
```

The CLI reads environment variables from the process environment. It does not auto-load `.env` by itself.

4. (For real paid requests) use a funded test wallet.

- `CLIENT_PRIVATE_KEY` must be the private key of the wallet you want to pay from.
- The wallet must be funded with the asset/network accepted by the server's `accepts` options.
- If the server example offers multiple options (for example Base Sepolia and Neo X), funding any accepted option can work.

5. Verify the server requirements without paying.

```bash
go run ./cmd/client inspect --url http://localhost:8080/paid/hello
```

6. Run a payment flow request.

```bash
go run ./cmd/client get --url http://localhost:8080/paid/hello
```

If configured correctly for real payment (key loaded + funded wallet + server settlement working), you should see:

- initial request
- `402 Payment Required`
- selected requirements
- retry with payment header
- final `200 OK`

Without those prerequisites, `200 OK` is not expected.

## Useful Commands

Inspect only (no payment attempt):

```bash
go run ./cmd/client inspect --url http://localhost:8080/paid/hello
```

Dry run (parse + policy check, but do not sign/retry):

```bash
go run ./cmd/client get --url http://localhost:8080/paid/hello --dry-run
```

Disable payment attempt entirely:

```bash
go run ./cmd/client get --url http://localhost:8080/paid/hello --no-pay
```

## Expected Outcomes by Mode

`inspect`:

- Always does a no-pay flow and stops at `402 Payment Required`
- Useful to confirm what the server is asking for before attempting payment

`get --dry-run`:

- Parses and validates payment requirements, but does not sign or retry
- Stays at `402 Payment Required`

`get --no-pay`:

- Does not attempt payment at all
- Stays at `402 Payment Required`

`get` (normal mode):

- With `CLIENT_PRIVATE_KEY` exported and a compatible funded wallet, attempts sign + retry and can return `200 OK`
- Without `CLIENT_PRIVATE_KEY`, payment signing is unavailable and paid flow cannot complete

`Body: null` on a `402` response can still be valid. Some servers communicate requirements primarily through the `PAYMENT-REQUIRED` header while using an empty or null JSON body.

POST example:

```bash
go run ./cmd/client post --url http://localhost:8080/paid/echo --body '{"message":"hello"}'
```

Show version:

```bash
go run ./cmd/client version
```

## Environment Variables

| Variable | Description | Default |
| -------- | ----------- | ------- |
| `CLIENT_LOG_LEVEL` | Logging level (`debug`, `info`, `warn`, `error`) | `info` |
| `CLIENT_LOG_JSON` | JSON log output | `false` |
| `CLIENT_PRIVATE_KEY` | EVM private key for signing | - |
| `CLIENT_MAX_AMOUNT` | Max allowed payment amount (smallest unit). Empty means no cap. | - |
| `CLIENT_ALLOWED_ASSET` | Comma-separated allowlist of assets | - |
| `CLIENT_ALLOWED_CHAIN_ID` | Allowed chain/network ID (CAIP-2 or numeric) | - |
| `CLIENT_ALLOWED_PAY_TO` | Comma-separated allowlist of recipients | - |
| `CLIENT_PREFERRED_NETWORKS` | CAIP-2 preferred networks, comma-separated | - |
| `CLIENT_PREFERRED_ASSETS` | Preferred asset addresses, comma-separated | - |
| `CLIENT_PREFERRED_TRANSFER_METHODS` | Preferred methods (`eip3009`, `permit2`) | - |
| `CLIENT_SELECTION_STRATEGY` | `server-order` or `preference-first` | `server-order` |
| `CLIENT_TIMEOUT` | HTTP timeout | `30s` |
| `CLIENT_DRY_RUN` | Parse 402 but do not sign/retry | `false` |
| `CLIENT_NO_PAY` | Never attempt payment flow | `false` |
| `CLIENT_RPC_URL` | Default EVM JSON-RPC URL for on-chain reads and Permit2 approvals | - |
| `CLIENT_RPC_EIP155_<CHAIN_ID>` | Per-network JSON-RPC override, e.g. `CLIENT_RPC_EIP155_84532` | - |

These defaults are the application's built-in defaults when no environment variable is set.
The checked-in `.env.example` is an opinionated Base Sepolia / Neo X example profile, so some copied values intentionally differ from the empty built-in defaults above.

### Buyer Policy Configuration

This is the core buyer control surface for deciding **what payments are allowed**.
`CLIENT_MAX_AMOUNT` has no default; leave it empty for no amount ceiling, or set it explicitly to enforce a cap.

How each payment option is evaluated:

1. The server sends one or more `accepts` options.
2. The client orders options based on `CLIENT_SELECTION_STRATEGY`.
3. Each option is checked against local policy using `CLIENT_MAX_AMOUNT`, `CLIENT_ALLOWED_ASSET`, `CLIENT_ALLOWED_CHAIN_ID`, and `CLIENT_ALLOWED_PAY_TO`.
4. The first option that passes policy is signed and retried.
5. If none pass, payment is refused with per-option rejection reasons.

Example: Strict allowlist (recommended for production buyers)

```bash
CLIENT_MAX_AMOUNT=<max_amount_smallest_unit>
CLIENT_ALLOWED_CHAIN_ID=eip155:84532
CLIENT_ALLOWED_ASSET=0x036CbD53842c5426634e7929541eC2318f3dCF7e
CLIENT_ALLOWED_PAY_TO=0x1111111111111111111111111111111111111111
CLIENT_SELECTION_STRATEGY=server-order
```

Example: Accept multiple sellers/assets, prefer one network/asset first

```bash
CLIENT_ALLOWED_CHAIN_ID=
CLIENT_ALLOWED_ASSET=0x036CbD53842c5426634e7929541eC2318f3dCF7e,0xABCDEF0000000000000000000000000000000001
CLIENT_ALLOWED_PAY_TO=0x1111111111111111111111111111111111111111,0x2222222222222222222222222222222222222222
CLIENT_PREFERRED_NETWORKS=eip155:12227332,eip155:84532
CLIENT_PREFERRED_ASSETS=0xABCDEF0000000000000000000000000000000001,0x036CbD53842c5426634e7929541eC2318f3dCF7e
CLIENT_SELECTION_STRATEGY=preference-first
```

Example: Safe validation before enabling payment

```bash
go run ./cmd/client get --url http://localhost:8080/paid/hello --dry-run --verbose
```

Use this to verify exactly which option would be selected (and why others are rejected) before running paid mode.

### Multi-Option Selection

When a server returns multiple payment options in its `402 Payment Required` response (via the `accepts` array), the client evaluates **all** options against local policy and selects the best acceptable one.

**Selection strategies:**

- `server-order` (default): Evaluate options in the server-provided order; select the first that passes policy. Backward compatible with single-option servers.
- `preference-first`: Reorder candidates by preference score (network > asset > transfer method) before applying policy. Useful when you want to prefer e.g., Neo X xGAS over Base Sepolia USDC.

**Example: Prefer Neo X xGAS**

```bash
CLIENT_PREFERRED_NETWORKS=eip155:47763
CLIENT_PREFERRED_ASSETS=0xABCDEF0000000000000000000000000000000001
CLIENT_SELECTION_STRATEGY=preference-first
```

**Diagnostics:** The client logs all offered options, the selected option, and per-option rejection reasons for any rejected candidates. Use `--verbose` or `CLIENT_LOG_LEVEL=debug` to see full details.

**CAIP-2 normalization:** Bare numeric chain IDs (e.g., `84532`) are automatically converted to CAIP-2 format (`eip155:84532`) in both `CLIENT_ALLOWED_CHAIN_ID` and `CLIENT_PREFERRED_NETWORKS`.

### Permit2 and RPC Configuration

Some payment options use `permit2` instead of `eip3009`. For Permit2 payments, the client may need to perform an on-chain allowance check and send an approval transaction before retrying the paid request.

Set `CLIENT_RPC_URL` to a default EVM JSON-RPC endpoint, or set a per-network override using the CAIP-2 network converted to an environment variable suffix:

```bash
CLIENT_RPC_URL=https://sepolia.base.org
CLIENT_RPC_EIP155_84532=https://sepolia.base.org
CLIENT_RPC_EIP155_12227332=https://neoxt4seed1.ngd.network/
```

Per-network overrides take precedence over `CLIENT_RPC_URL`.

## CLI

```bash
x402-client get --url <URL> [--dry-run] [--no-pay] [--verbose] [--timeout <duration>]
```

## Build and Test

Run tests:

```bash
make test
```

Build binary:

```bash
make build
```

Format and vet:

```bash
make check
```

## End-to-End Testing

E2E tests verify the full x402 payment flow against a real running `x402-go-server-example` instance. They are **skipped by default** and do not run as part of `go test ./...` unless the required environment variables are set.

### Prerequisites

- A running `x402-go-server-example` server exposing:
  - `GET /healthz` (unpaid)
  - `GET /info` (unpaid)
  - `GET /paid/hello` (paid, multi-option: Base Sepolia + Neo X)
  - `POST /paid/echo` (paid)
- For paid tests: a funded test private key compatible with the server's facilitator/settlement setup

### Running E2E Tests

**Unpaid endpoint tests only** (no private key needed):

```bash
X402_E2E_SERVER_URL=http://localhost:8080 make test-e2e
```

**Full E2E tests** (including paid flows):

```bash
X402_E2E_SERVER_URL=http://localhost:8080 \
X402_E2E_PRIVATE_KEY=0x... \
make test-e2e
```

**With multi-option preference verification:**

```bash
X402_E2E_SERVER_URL=http://localhost:8080 \
X402_E2E_PRIVATE_KEY=0x... \
CLIENT_PREFERRED_NETWORKS=eip155:12227332 \
make test-e2e
```

### What's Tested

| Test | Requires | Verifies |
| ---- | -------- | -------- |
| `TestE2E_Healthz` | Server URL | Unpaid endpoint returns 200 |
| `TestE2E_Info` | Server URL | Pricing metadata present |
| `TestE2E_PaidHello` | Server URL + Key | Full 402 → sign → retry → 200 flow |
| `TestE2E_PaidEcho_BodyPreserved` | Server URL + Key | POST body preserved across retry |
| `TestE2E_MultiOption_PreferenceSelection` | Server URL + Key | Neo X selected when preferred |
| `TestE2E_PolicyRejection_Fallback` | Server URL + Key | Fallback to Neo X when Base Sepolia rejected |
| `TestE2E_NoAcceptableOptions` | Server URL + Key | Explicit error with rejection reasons |

### Important Notes

- Normal `go test ./...` passes without a running server (E2E tests skip)
- No private keys should be committed to the repository
- The server must be started separately; E2E tests do not start it
- Paid tests require a test account compatible with the server's settlement setup

## Project Structure

```text
x402-go-client-example/
├── cmd/
│   └── client/
│       └── main.go
├── internal/
│   ├── cli/
│   ├── config/
│   ├── httpclient/
│   ├── logging/
│   ├── payment/
│   │   ├── policy/
│   │   └── selection/
│   ├── permit2/
│   ├── signer/
│   ├── version/
│   └── x402adapter/
├── docs/
├── test/
├── .env.example
├── Makefile
└── README.md
```

## Documentation

- [Architecture](docs/architecture.md)
- [Flow](docs/flow.md)

## License

MIT License
