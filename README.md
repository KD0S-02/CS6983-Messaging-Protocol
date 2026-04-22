# CS6983 Messaging Protocol

A replicated messaging/storage protocol implemented in Go. The code is broken down into internal packages for the wire protocol, SSD storage nodes, network clients, gateway coordination, a small runnable local cluster, and regression testing.

The project can be used in two ways:

- run the package tests with `go test ./...`
- run a local development cluster with `go run ./cmd/devcluster -http 127.0.0.1:18080`

## Requirements

- Go `1.25.5` or the version listed in `go.mod`
- Git
- A terminal such as PowerShell, Command Prompt, Git Bash, or WSL

Check your Go install:

```powershell
go version
```

## Getting the code

Clone the repository:

```powershell
git clone https://github.com/KD0S-02/CS6983-Messaging-Protocol.git
```

Go into the repository root:

```powershell
cd C:\Users\prana\OneDrive\Desktop\CS6983-Messaging-Protocol
```

Most commands in this README should be run from that root folder.

## Quick check

Run the whole test suite:

```powershell
go test ./...
```

Run with verbose output:

```powershell
go test ./... -v
```

Run one package:

```powershell
go test ./internal/proto
```

Run one test:

```powershell
go test ./internal/gateway -run TestEndToEndPutOnGateway1ReadableFromGateway2 -v
```

Run the race detector:

```powershell
go test ./... -race
```

Generate coverage:

```powershell
go test ./... -cover
```

Generate a coverage profile:

```powershell
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Running the local dev cluster

The repository includes a runnable development command at:

```text
cmd/devcluster/main.go
```

This command starts:

- 3 local SSD servers
- 2 gateway peer servers by default
- an HTTP API for basic PUT and GET requests

Use port `18080` for the HTTP API to avoid common conflicts with other local services on port `8080`.

### Terminal 1: start the cluster

Open a PowerShell terminal and run:

```powershell
cd C:\Users\prana\OneDrive\Desktop\CS6983-Messaging-Protocol

go run ./cmd/devcluster -http 127.0.0.1:18080
```

Leave this terminal open while testing. The server is running correctly if it stays open and shows logs similar to:

```text
ssd-0 listening on 127.0.0.1:9100 datadir=data\devcluster\ssd-0
ssd-1 listening on 127.0.0.1:9101 datadir=data\devcluster\ssd-1
ssd-2 listening on 127.0.0.1:9102 datadir=data\devcluster\ssd-2
gateway-1 peer listener on 127.0.0.1:9201
gateway-2 peer listener on 127.0.0.1:9202
HTTP API listening on http://127.0.0.1:18080
```

### Terminal 2: test the API

Open a second PowerShell terminal and run the commands below from anywhere.

Check health:

```powershell
curl.exe http://127.0.0.1:18080/healthz
```

Expected response:

```json
{"status":"ok"}
```

Write a value through gateway 1:

```powershell
curl.exe -X PUT --data-binary "hello world" http://127.0.0.1:18080/gateways/1/kv/demo
```

Expected response:

```json
{"bytes":11,"gateway":1,"key":"demo","status":"ok"}
```

Read the value back through gateway 1:

```powershell
curl.exe http://127.0.0.1:18080/gateways/1/kv/demo
```

Expected response:

```text
hello world
```

Read the same value through gateway 2:

```powershell
curl.exe http://127.0.0.1:18080/gateways/2/kv/demo
```

Expected response:

```text
hello world
```

Show the local cluster configuration:

```powershell
curl.exe http://127.0.0.1:18080/gateways
```

### Useful devcluster flags

Run with 3 gateways:

```powershell
go run ./cmd/devcluster -http 127.0.0.1:18080 -gateways 3
```

Run with SSD latency simulation:

```powershell
go run ./cmd/devcluster -http 127.0.0.1:18080 -latency-ms 100 -jitter-ms 50
```

Run with injected SSD failures:

```powershell
go run ./cmd/devcluster -http 127.0.0.1:18080 -fail-rate 0.1
```

Use a different data directory:

```powershell
go run ./cmd/devcluster -http 127.0.0.1:18080 -data ./tmp/devcluster
```

Use completely different SSD, peer, and HTTP ports:

```powershell
go run ./cmd/devcluster `
  -http 127.0.0.1:18080 `
  -ssd-addrs 127.0.0.1:19100,127.0.0.1:19101,127.0.0.1:19102 `
  -peer-addrs 127.0.0.1:19201,127.0.0.1:19202
```

## Formatting

Format all Go files before committing:

```powershell
gofmt -w internal/proto internal/ssd internal/transport internal/gateway cmd/devcluster
```

Then rerun tests:

```powershell
go test ./...
```

## Repository layout

```text
.
├── cmd
│   └── devcluster
├── go.mod
├── Makefile
├── README.md
└── internal
    ├── gateway
    ├── proto
    ├── ssd
    └── transport
```

The code is intended to be used by this module rather than directly imported by external modules. This is why the project makes use of `internal/` packages.

## File map

### Root

- `go.mod` — module name and Go version for the project.
- `Makefile` — placeholder make file for future project commands.
- `README.md` — setup, run, test, and file guide for the project.

### `cmd/devcluster`

- `main.go` — Starts a local development cluster with SSD servers, gateway peer servers, and an HTTP API for testing PUT and GET requests.

### `internal/proto`

- `messages.go` — Contains the shared protocol structs for gateway phases, SSD reads/writes, and replica locations.
- `framing.go` — Holds the TCP frame encoding/decoding using a 1-byte message type, 4-byte payload length, and gob payload.
- `errors.go` — Contains the shared protocol-level errors.
- `framing_test.go` — Holds the basic protocol round-trip tests and truncated payload checks.
- `framing_edge_cases_test.go` — Holds the malformed payload, EOF, multiple-message stream, and gob encode error tests.

### `internal/ssd`

- `ssd.go` — Holds the file-backed SSD implementation with append, read, block files, and LBA recovery.
- `server.go` — Contains the TCP server that exposes SSD append/read operations using the protocol package.
- `config.go` — Holds the latency, jitter, and failure-injection settings for SSD operations.
- `errors.go` — Holds the SSD-specific errors such as missing blocks and injected failures.
- `ssd_test.go` — Contains the basic SSD append/read, restart recovery, and not-found tests.
- `ssd_edge_cases_test.go` — Contains the binary data, empty values, concurrent appends, malformed blocks, and recovery edge cases.

### `internal/transport`

- `ssd_client.go` — Holds the gateway-side client for sending append/read requests to SSD servers.
- `peer_client.go` — Holds the gateway-side client for sending Phase1 and Phase2 messages to peer gateways.
- `pool.go` — Contains the small TCP connection pool helper.
- `ssd_client_test.go` — Used to verify SSD client behavior around application-level errors.
- `transport_edge_cases_test.go` — Contains the integration tests for SSD client/server traffic, reconnects, peer sends, and connection pool behavior.

### `internal/gateway`

- `gateway.go` — Holds the gateway struct, constructor, sequence counter, SSD clients, peer clients, index, and outstanding-write tracker.
- `handler.go` — Used for PUT/GET handling, peer Phase1/Phase2 handling, failed-write cleanup, and replica fallback reads.
- `index.go` — Contains the in-memory key-to-location index with version comparison using sequence number and gateway ID.
- `outstanding.go` — Holds the pending-write tracking, waiter registration, completion handling, and timeout cleanup.
- `peer_server.go` — Contains the TCP listener for incoming peer gateway messages.
- `gateway_e2e_test.go` — Holds the end-to-end gateway tests for PUT/GET, peer propagation, pending reads, failures, and replica fallback.
- `gateway_edge_cases_test.go` — Contains the focused gateway tests for stale metadata, multiple waiters, out-of-order phases, and indexing behavior.

## Test coverage notes

The tests written and covered in this RPC protocol are:

- protocol framing and gob payload handling
- SSD append/read and recovery behavior
- SSD injected failures and malformed block files
- SSD client retry behavior
- peer client Phase1/Phase2 sends
- connection pool behavior
- gateway PUT/GET behavior
- peer metadata propagation
- pending-write blocking/unblocking
- failed-write cleanup
- same-key writer convergence
- replica fallback reads

## Running package-specific tests

Protocol tests:

```powershell
go test ./internal/proto -v
```

SSD tests:

```powershell
go test ./internal/ssd -v
```

Transport tests:

```powershell
go test ./internal/transport -v
```

Gateway tests:

```powershell
go test ./internal/gateway -v
```

Devcluster compile check:

```powershell
go test ./cmd/devcluster
```

## Development workflow

A simple workflow for local changes:

```powershell
cd C:\Users\prana\OneDrive\Desktop\CS6983-Messaging-Protocol

git status

gofmt -w internal/proto internal/ssd internal/transport internal/gateway cmd/devcluster
go test ./...

git add .
git commit -m "Describe your change"
git push origin HEAD
```

If you only want to stage specific files, use `git add path/to/file.go` instead of `git add .`.

## Git ignore note

The devcluster command writes local SSD block files under `data/`. Coverage commands can also create `coverage.out`.

It is recommended to keep these out of Git:

```gitignore
data/
coverage.out
```

## Troubleshooting

### `go test` tries to download another Go toolchain

The project declares its Go version in `go.mod`. If your installed Go version does not match and automatic toolchain download is unavailable, install the version listed in `go.mod` or use a machine that has it available.

### `package ... is not in std`

Make sure you are running commands from the repository root:

```powershell
cd C:\Users\prana\OneDrive\Desktop\CS6983-Messaging-Protocol
```

Then run:

```powershell
go test ./...
```

### Port `8080` gives HTML `404 Not Found` or `405 Method Not Allowed`

If you see an HTML response like this:

```html
<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
```

then you are probably hitting another service on port `8080`, not this Go devcluster API. This project's API returns JSON errors, not that HTML page.

Use port `18080` instead:

```powershell
go run ./cmd/devcluster -http 127.0.0.1:18080
```

Then test with:

```powershell
curl.exe http://127.0.0.1:18080/healthz
```

### `bind: An attempt was made to access a socket in a way forbidden by its access permissions`

This means the port is already in use or blocked. The easiest fix is to run the HTTP API on port `18080`:

```powershell
go run ./cmd/devcluster -http 127.0.0.1:18080
```

To check what is using port `8080`, run:

```powershell
netstat -ano | findstr :8080
```

The last number in the result is the process ID. To inspect it:

```powershell
tasklist /FI "PID eq 12345"
```

Replace `12345` with the actual process ID from `netstat`.

### Tests hang or fail around networking

The tests use local TCP listeners. Rerun the package with `-v` to see where it stopped:

```powershell
go test ./internal/gateway -v
```

If needed, run one test at a time with `-run`.
