# AGENTS.md — wspulse/testserver

Shared integration test server for non-Go wspulse client integration tests. Read this file before editing any file in `testserver/`.

## Project Overview

`testserver` is a standalone Go `main` binary (`github.com/wspulse/testserver`) that exposes two local TCP ports:

- **WebSocket port** — echo server backed by `wspulse/hub`. Behaviour is controlled by URL query parameters.
- **Control port** — HTTP API for test orchestration (`/health`, `/kick`, `/shutdown`, `/restart`).

On startup it prints `READY:<ws_port>:<control_port>` to stderr; client test harnesses parse this line to discover both ports. Depends on `github.com/wspulse/hub`.

## File Index

| File           | Purpose                                                    |
| -------------- | ---------------------------------------------------------- |
| `main.go`      | `testServer` struct, dual-port startup, all HTTP handlers  |
| `main_test.go` | Integration tests covering all endpoints and WS behaviour  |
| `Makefile`     | `fmt`, `lint`, `test`, `build`, `check`, `tidy`, `clean`   |

## Query-Parameter Protocol (WebSocket port)

| Parameter      | Effect                                                              |
| -------------- | ------------------------------------------------------------------- |
| `?reject=1`    | `ConnectFunc` returns an error → HTTP 401                           |
| `?room=<id>`   | Assigns connection to room `<id>` (default: `"test"`)               |
| `?id=<id>`     | Sets `connectionID` (default: auto-generated UUID)                  |
| `?ignore_pings=1` | Bypasses wspulse/hub; raw echo that suppresses Pong replies   |

## Development Workflow

```bash
make fmt    # format (gofmt + goimports)
make lint   # vet + golangci-lint
make test   # tests with race detector
make check  # fmt + lint + test (pre-commit gate)
make build  # build the testserver binary
make tidy   # tidy module dependencies
make clean  # remove build artifacts and test cache
```

## Conventions

- **Go style**: `gofmt`/`goimports`, snake_case filenames, `if err != nil` error handling, secrets from env vars only.
- **Naming**: interface names use full words; variable names follow standard Go style.
- **Error format**: `fmt.Errorf("testserver: <context>: %w", err)`.
- **Markdown**: no emojis in documentation files.
- **Git branch strategy**: `feat/<name>`, `refactor/<name>`, `bugfix/<name>`, `fix/<name>`, `chore/<name>`. Never push directly to `develop` or `main`.
- **Commit messages**: English only, one logical change per commit. Follow the structured format in `.github/instructions/commit-message-instructions.md`.
- **Dependency policy**: prefer stdlib; justify any new external dependency in the PR description.

## Critical Rules

1. **Read before write** — read `main.go` and `README.md` in full before any edit.
2. **Minimal changes** — one concern per edit; no drive-by refactors.
3. **No hardcoded secrets** — all configuration via environment variables.
4. **Goroutine lifecycle** — every goroutine must have an explicit exit condition; `close()` must not leak goroutines.
5. **Mutex discipline** — all `testServer` state mutations must be guarded by `ts.mu`. Never hold the mutex across blocking calls.
6. **Port stability** — `POST /restart` must rebind to the same WebSocket port so client URLs remain valid.
7. **READY protocol** — the `READY:<ws_port>:<control_port>` line on stderr is a contract consumed by client test harnesses. Do not change its format without updating all client test harnesses.
8. **STOP — test first, fix second** — for any bug fix, follow this sequence without skipping steps:
    1. Write a failing test that reproduces the bug.
    2. Confirm it fails.
    3. Fix the production code.
    4. Confirm the test passes.
    5. Run `make check`.
9. **STOP — before every commit, verify this checklist**:
    1. Run `make check` (fmt → lint → test) and confirm it passes.
    2. Commit message follows the structured format.
    3. This commit contains exactly one logical change.
10. **Accuracy** — do not make assumptions. Ask the user when clarification is needed.
11. **Language consistency** — respond in Traditional Chinese when the user writes in Traditional Chinese; otherwise respond in English.

## Session Protocol

> Files under `doc/local/` are git-ignored and must **never** be committed.
> This includes plan files (`doc/local/plan/`) and the AI learning log (`doc/local/ai-learning.md`).

### Start of every session — MANDATORY

**Do these steps before writing any code:**

1. Read `doc/local/ai-learning.md` **in full** to recall past mistakes. If the file is missing or empty, create it with the table header (see format below) before proceeding.
2. Check `doc/local/plan/` for any in-progress plan and read it fully.

### During feature work

For any new feature or multi-file fix: save a plan to `doc/local/plan/<feature-name>.md` **before starting**. Keep it updated with completed steps throughout the session.

### End of every session — MANDATORY

**Before closing the session, complete this checklist without exception:**

1. Append at least one entry to `doc/local/ai-learning.md` — **even if no mistakes were made**. Record what you confirmed, what technique worked, or what you observed. An empty file is a sign of non-compliance.
2. Update any in-progress plan in `doc/local/plan/` to reflect completed steps.
3. Verify `make check` passes.

**Entry format** for `doc/local/ai-learning.md`:

```
| Date       | Issue or Learning | Root Cause | Prevention Rule |
| ---------- | ----------------- | ---------- | --------------- |
| YYYY-MM-DD | <what happened or what you learned> | <why it happened> | <how to avoid it next time> |
```

**Writing to `ai-learning.md` is not optional. It is the primary cross-session improvement mechanism. An empty file proves the session protocol was ignored.**
