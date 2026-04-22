# CS6983 Messaging Protocol

A replicated messaging/storage protocol implemented in Go. The project is split into packages for the wire protocol, SSD-style storage nodes, transport clients, gateway coordination, a runnable local development cluster, and regression tests.

This repository can be used in two ways:

1. Run the full test suite with `go test ./...`.
2. Start a local dev cluster with `cmd/devcluster` and test the system through a small HTTP API.

## Environment used

The project was developed and tested with the following setup:

- Go version listed in `go.mod` (`1.25.5` in this repository)
- Git
- Windows with PowerShell
- Local TCP ports for the dev cluster:
  - HTTP API: `127.0.0.1:18080`
  - SSD servers: `127.0.0.1:9100`, `127.0.0.1:9101`, `127.0.0.1:9102`
  - Gateway peer listeners: `127.0.0.1:9201`, `127.0.0.1:9202`

Other terminals such as Command Prompt, Git Bash, WSL, macOS Terminal, or Linux shell should also work, but the examples below use PowerShell syntax.

Check your Go install:

```powershell
go version
```

Check Git:

```powershell
git --version
```

## Getting the code

Clone the repository:

```powershell
git clone https://github.com/KD0S-02/CS6983-Messaging-Protocol.git
```

Go into the project root:

Most commands in this README should be run from the repository root.

## Quick test

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

Generate a coverage profile and open the HTML view:

```powershell
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Building and running the system

The runnable entry point is:

```text
cmd/devcluster/main.go
```

It starts:

- 3 local SSD servers
- 2 gateway peer servers by default
- an HTTP API for client-facing `PUT` and `GET` requests

### Option 1: run directly with Go

Use port `18080` for the HTTP API. This avoids common conflicts with other services on port `8080`.

Terminal 1:

```powershell
go run ./cmd/devcluster -http 127.0.0.1:18080
```

Leave Terminal 1 running. You should see logs for the SSD servers, gateway peer listeners, and HTTP API.

Terminal 2:

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

Read the value through gateway 1:

```powershell
curl.exe http://127.0.0.1:18080/gateways/1/kv/demo
```

Read the same value through gateway 2:

```powershell
curl.exe http://127.0.0.1:18080/gateways/2/kv/demo
```

Show cluster information:

```powershell
curl.exe http://127.0.0.1:18080/gateways
```

Stop the cluster by pressing `Ctrl + C` in Terminal 1.

### Option 2: build an executable

Create a local `bin` directory:

```powershell
New-Item -ItemType Directory -Force .\bin
```

Build the dev cluster command:

```powershell
go build -o .\bin\devcluster.exe ./cmd/devcluster
```

Run it:

```powershell
.\bin\devcluster.exe -http 127.0.0.1:18080
```

Then use the same `curl.exe` commands from a second terminal.

## Makefile

The `Makefile` is a shortcut file for the most common commands. It wraps the same commands used in this README, including formatting, tests, coverage, running the dev cluster, API checks, cleanup, and Git workflow commands.

Show all available targets:

```powershell
make help
```

Common targets:

```powershell
make fmt
make test
make test-v
make test-race
make cover
make cover-html
```

Run the local dev cluster on `127.0.0.1:18080`:

```powershell
make run
```

In a second terminal, test the API:

```powershell
make api-health
make api-put VALUE="hello world" KEY=demo
make api-get-gw1 KEY=demo
make api-get-gw2 KEY=demo
make api-info
```

Run the whole API demo sequence:

```powershell
make api-demo
```

Run package-specific tests:

```powershell
make test-proto
make test-ssd
make test-transport
make test-gateway
make test-devcluster
```

Run one specific test:

```powershell
make test-one PKG=./internal/gateway TEST=TestEndToEndPutOnGateway1ReadableFromGateway2
```

Clean generated runtime files:

```powershell
make clean
```

Git helper targets are also included:

```powershell
make git-status
make git-add
make git-commit MSG="Describe your change"
make git-pull
make git-push
```

If `make` is not installed on your machine, use the direct `go`, `curl.exe`, and `git` commands shown in the README instead.

## Design overview

The main design decision was to keep the project split into small internal packages instead of putting everything into one large file. Each package has one job:

- `internal/proto` defines the message structs and frame format used over TCP.
- `internal/ssd` implements the local file-backed SSD simulation and SSD TCP server.
- `internal/transport` contains clients for gateway-to-SSD and gateway-to-gateway communication.
- `internal/gateway` contains the main replicated write/read coordination logic.
- `cmd/devcluster` wires the packages together into a runnable local system.

The storage layer is intentionally simple. Each SSD stores values in block files and returns a logical block address. The gateway layer owns the key-to-location index and is responsible for coordinating writes with peer gateways.

A write through `HandlePUT` does the following:

1. assigns a gateway-local sequence number,
2. sends Phase1 to peer gateways to mark the key as pending,
3. appends the value to all three SSD replicas,
4. updates the local gateway index,
5. sends Phase2 to peers with the final replica locations.

A read through `HandleGET` checks whether a peer write is pending, waits for Phase2 if needed, looks up the key in the local index, and then reads from one of the available replicas.

A few important design choices:

- TCP is used directly instead of HTTP or gRPC for internal gateway/SSD traffic.
- Messages are encoded with Go `gob` and wrapped in a small custom frame.
- SSD nodes are dumb storage nodes; gateways manage metadata and coordination.
- Values are replicated to three SSD servers.
- Reads can fall back to another replica if the first replica fails.
- Failed writes send a cleanup Phase2 message so peer gateways do not stay stuck in a pending state.
- Gateway metadata is in memory for this prototype; SSD block files are persisted under `data/` during devcluster runs.

## Repository layout

```text
.
├── cmd
│   └── devcluster
│       ├── main.go
│       └── main_test.go
├── go.mod
├── Makefile
├── README.md
└── internal
    ├── gateway
    ├── proto
    ├── ssd
    └── transport
```

The code is intended to be used inside this module rather than directly imported by external modules. This is why the project uses `internal/` packages.

## File map

### Root

- `go.mod` — module name and Go version for the project.
- `Makefile` — shortcut commands for testing, formatting, running the dev cluster, API checks, cleanup, and Git workflow.
- `README.md` — setup, build, run, testing, design, and troubleshooting notes.

### `cmd/devcluster`

- `main.go` — starts local SSD servers, gateway peer servers, wires peers together, and exposes the HTTP API.
- `main_test.go` — tests command helper functions, path parsing, cluster info, and basic HTTP routes.

### `internal/proto`

- `messages.go` — contains the shared protocol structs for gateway phases, SSD reads/writes, and replica locations.
- `framing.go` — holds the TCP frame encoding/decoding using a 1-byte message type, 4-byte payload length, and gob payload.
- `errors.go` — contains the shared protocol-level errors.
- `framing_test.go` — holds the basic protocol round-trip tests and truncated payload checks.
- `framing_edge_cases_test.go` — holds the malformed payload, EOF, multiple-message stream, and gob encode error tests.

### `internal/ssd`

- `ssd.go` — holds the file-backed SSD implementation with append, read, block files, and LBA recovery.
- `server.go` — contains the TCP server that exposes SSD append/read operations using the protocol package.
- `config.go` — holds the latency, jitter, and failure-injection settings for SSD operations.
- `errors.go` — holds the SSD-specific errors such as missing blocks and injected failures.
- `ssd_test.go` — contains the basic SSD append/read, restart recovery, and not-found tests.
- `ssd_edge_cases_test.go` — contains the binary data, empty values, concurrent appends, malformed blocks, and recovery edge cases.

### `internal/transport`

- `ssd_client.go` — holds the gateway-side client for sending append/read requests to SSD servers.
- `peer_client.go` — holds the gateway-side client for sending Phase1 and Phase2 messages to peer gateways.
- `pool.go` — contains the small TCP connection pool helper.
- `ssd_client_test.go` — verifies SSD client behavior around application-level errors.
- `transport_edge_cases_test.go` — contains the integration tests for SSD client/server traffic, reconnects, peer sends, and connection pool behavior.

### `internal/gateway`

- `gateway.go` — holds the gateway struct, constructor, sequence counter, SSD clients, peer clients, index, and outstanding-write tracker.
- `handler.go` — handles PUT/GET requests, peer Phase1/Phase2 messages, failed-write cleanup, and replica fallback reads.
- `index.go` — contains the in-memory key-to-location index with version comparison using sequence number and gateway ID.
- `outstanding.go` — holds pending-write tracking, waiter registration, completion handling, and timeout cleanup.
- `peer_server.go` — contains the TCP listener for incoming peer gateway messages.
- `gateway_e2e_test.go` — holds end-to-end gateway tests for PUT/GET, peer propagation, pending reads, failures, and replica fallback.
- `gateway_edge_cases_test.go` — contains focused gateway tests for stale metadata, multiple waiters, out-of-order phases, and indexing behavior.

## Testing performed

The test suite covers unit tests, edge-case tests, and end-to-end tests across all main packages.

Areas tested:

- protocol framing and gob payload handling
- multiple messages on the same stream
- truncated and malformed payloads
- SSD append/read behavior
- SSD LBA recovery after restart
- binary values, empty values, and malformed block files
- SSD injected failures
- SSD client retry behavior
- peer client Phase1/Phase2 sends
- connection pool reuse/discard behavior
- gateway PUT/GET behavior
- peer metadata propagation
- pending-write blocking and unblocking
- failed-write cleanup
- same-key writer convergence
- stale metadata handling
- out-of-order Phase1/Phase2 cases
- replica fallback reads
- devcluster helper functions and HTTP routes

Run all tests:

```powershell
go test ./...
```

Run package-specific tests:

```powershell
go test ./cmd/devcluster -v
go test ./internal/proto -v
go test ./internal/ssd -v
go test ./internal/transport -v
go test ./internal/gateway -v
```

Expected final result:

```text
ok   github.com/KD0S-02/cs6983-messaging-protocol/cmd/devcluster
ok   github.com/KD0S-02/cs6983-messaging-protocol/internal/gateway
ok   github.com/KD0S-02/cs6983-messaging-protocol/internal/proto
ok   github.com/KD0S-02/cs6983-messaging-protocol/internal/ssd
ok   github.com/KD0S-02/cs6983-messaging-protocol/internal/transport
```

Some of the tests were originally written to expose bugs in the prototype. The fixes added afterward addressed these behaviors:

- SSD client no longer retries server-side application errors as if they were network errors.
- Failed writes now clear peer pending-write state.
- Gateway reads can fall back to another replica.
- Outstanding-write waiters are cleaned up correctly.
- Out-of-order Phase messages and stale metadata are handled more safely.
- Same-key writes converge using sequence number and gateway ID comparison.

## Formatting

Format all Go files before committing:

```powershell
gofmt -w internal/proto internal/ssd internal/transport internal/gateway cmd/devcluster
```

Then rerun tests:

```powershell
go test ./...
```

The Makefile shortcut is:

```powershell
make fmt
make test
```
## Troubleshooting

### `go test` tries to download another Go toolchain

The project declares its Go version in `go.mod`. If your installed Go version does not match and automatic toolchain download is unavailable, install the version listed in `go.mod` or use a machine that has it available.

### `package ... is not in std`

Make sure you are running commands from the repository root:

Then run:

```powershell
go test ./...
```

### Port `8080` is already being used

The dev cluster can run on `18080`, which is the recommended local port for this README:

```powershell
go run ./cmd/devcluster -http 127.0.0.1:18080
```

To check what is using port `8080`:

```powershell
netstat -ano | findstr :8080
```

To check port `18080`:

```powershell
netstat -ano | findstr :18080
```

### `curl` returns an HTML 404 or 405 page

If the response starts with something like this:

```html
<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
```

then you are probably hitting another service, not this Go dev cluster. Make sure the dev cluster is running and that your `curl.exe` command uses the same port:

```powershell
curl.exe http://127.0.0.1:18080/healthz
```

### Tests hang or fail around networking

The tests use local TCP listeners. Rerun the package with `-v` to see where it stopped:

```powershell
go test ./internal/gateway -v
```

If needed, run one test at a time with `-run`.

### `data/` appears in `git status`

The dev cluster writes local SSD block files under `data/`. These are runtime files and should not be committed.

Add this to `.gitignore`:

```gitignore
data/
coverage.out
```

If `data/` was staged accidentally:

```powershell
git restore --staged data/
```
