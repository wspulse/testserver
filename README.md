# wspulse/testserver

[![CI](https://github.com/wspulse/testserver/actions/workflows/ci.yml/badge.svg)](https://github.com/wspulse/testserver/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Shared test server for wspulse non-Go client integration tests. Provides a WebSocket echo server with an HTTP control API for test orchestration.

**This is a test-only tool.** It is not intended for production use.

---

## Why

client-go tests reconnect/heartbeat scenarios by embedding `wspulse/server` in-process. Non-Go clients (Kotlin, TypeScript, Swift) run the server as an external process and need a way to trigger server-side actions (kick connections, shut down, restart) from their test harnesses.

---

## Architecture

The testserver listens on **two ports**, both bound to `127.0.0.1`:

| Port | Protocol | Purpose |
|------|----------|---------|
| WS port | WebSocket | Echo server — mirrors all inbound frames back to the sender |
| Control port | HTTP | Test orchestration — kick, shutdown, restart |

On startup it prints to stderr:

```
READY:<ws_port>:<control_port>
```

Client test harnesses parse this line to discover both ports.

---

## Quick Start

```bash
# Build
go build -o testserver .

# Run
./testserver
# stderr: READY:54321:54322
```

Or directly:

```bash
go run .
```

---

## WebSocket Behaviour

Connect to the WS port. All inbound frames are echoed back to the sender.

| Query Param | Effect |
|-------------|--------|
| `reject=1` | Connection rejected (HTTP 401) |
| `room=<id>` | Assigns connection to room (default: `"test"`) |
| `id=<id>` | Sets connectionID (default: auto-generated UUID) |

---

## Control API

All endpoints are on the control port. Responses are JSON.

### `GET /health`

Returns `200 OK` with `{"ok": true}`. Use to verify the control server is alive.

### `POST /kick?id=<connectionID>`

Kicks the specified connection. The WebSocket is closed server-side, triggering a transport drop on the client.

```bash
curl -X POST "http://127.0.0.1:<ctl_port>/kick?id=my-client"
# {"ok":true}
```

### `POST /shutdown`

Closes the WebSocket server and listener. All existing connections are dropped. New dial attempts will fail. The control port remains alive.

```bash
curl -X POST "http://127.0.0.1:<ctl_port>/shutdown"
# {"ok":true}
```

### `POST /restart`

Creates a new WebSocket server and rebinds to the **same WS port**. Client URLs remain valid.

```bash
curl -X POST "http://127.0.0.1:<ctl_port>/restart"
# {"ok":true}
```

---

## Integration Test Scenarios

| Scenario | Control API Usage |
|----------|-------------------|
| Auto-reconnect success | `POST /kick?id=X` — server stays up, client reconnects |
| Max retries exhausted | `POST /shutdown` — all dials fail |
| Close during reconnect | `POST /kick?id=X` then client calls `close()` |
| Shutdown + restart | `POST /shutdown` then `POST /restart` |

---

## Client Integration

Each client test harness:

1. Builds and spawns the testserver as a child process
2. Parses `READY:<ws>:<ctl>` from stderr
3. Uses the WS port for WebSocket connections
4. Uses the control port to orchestrate test scenarios
5. Kills the process on test teardown

---

## Development

```bash
make check    # fmt → lint → test (pre-commit gate)
make test     # run tests with race detector
make build    # compile binary
make fmt      # format source
make lint     # go vet + golangci-lint
```

Requires: Go 1.26+, [golangci-lint](https://golangci-lint.run/).

