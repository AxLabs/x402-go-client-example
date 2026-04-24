# x402-go-client-example

A Go **example client** for the buyer/caller side of the x402 payment flow.

This repository is intentionally paired with **`x402-go-server-example`**.
Use both repos together to run a complete local example of `402 Payment Required` -> payment authorization -> retry.

## What This Example Shows

- Calling a protected endpoint
- Handling `402 Payment Required`
- Parsing payment requirements
- Applying local payment policy checks
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
git clone https://github.com/bane-labs-org/x402-go-client-example.git
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

4. Verify the server requirements without paying.

```bash
go run ./cmd/client inspect --url http://localhost:8080/paid/hello
```

5. Run a payment flow request.

```bash
go run ./cmd/client get --url http://localhost:8080/paid/hello
```

If configured correctly, you should see:

- initial request
- `402 Payment Required`
- selected requirements
- retry with payment header
- final `200 OK`

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
| `CLIENT_MAX_AMOUNT` | Max allowed payment amount (smallest unit) | `1000000` |
| `CLIENT_ALLOWED_ASSET` | Comma-separated allowlist of assets | - |
| `CLIENT_ALLOWED_CHAIN_ID` | Allowed chain/network ID | - |
| `CLIENT_ALLOWED_PAY_TO` | Comma-separated allowlist of recipients | - |
| `CLIENT_TIMEOUT` | HTTP timeout | `30s` |
| `CLIENT_DRY_RUN` | Parse 402 but do not sign/retry | `false` |
| `CLIENT_NO_PAY` | Never attempt payment flow | `false` |

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
│   │   └── policy/
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
