# Copilot Instructions — wspulse/testserver

## Project Overview

wspulse/testserver is a **shared test server** for non-Go wspulse client integration tests. It exposes two local ports: a WebSocket echo server (via `wspulse/server`) and an HTTP control API for test orchestration. Module path: `github.com/wspulse/testserver`. Package name: `main`. Depends on `github.com/wspulse/server`.

## Architecture

- **`main.go`** — `testServer` struct with dual-port architecture:
  - **WebSocket port** — echo server with query-param-controlled behaviour (`?reject=1`, `?room=<id>`, `?id=<id>`).
  - **Control port** — HTTP API: `GET /health`, `POST /kick`, `POST /shutdown`, `POST /restart`.
  - Prints `READY:<ws_port>:<control_port>` to stderr on startup.
- **`main_test.go`** — Integration tests covering all control endpoints and WebSocket behaviour.

## Development Workflow

```bash
make fmt              # format (gofmt + goimports)
make lint             # vet + golangci-lint
make test             # unit tests with race detector
make check            # fmt + lint + test (pre-commit gate)
make build            # build the testserver binary
make tidy             # tidy module dependencies
make clean            # remove build artifacts and test cache
```

## Conventions

- **Go style**: `gofmt`/`goimports`, snake_case filenames, `if err != nil` error handling, secrets from env vars only.
- **Naming**:
  - **Interface names** must use full words — no abbreviations.
  - **Variable and parameter names** follow standard Go style: short receivers, idiomatic short names for local scope.
- **Markdown**: no emojis in documentation files.
- **Git**:
  - Follow the commit message rules in [commit-message-instructions.md](instructions/commit-message-instructions.md).
  - All commit messages in English.
  - Each commit must represent exactly one logical change.
  - Before every commit, run `make check` (runs fmt -> lint -> test in order).
  - **Branch strategy**: never push directly to `develop` or `main`.
    - `feature/<name>` — new feature
    - `refactor/<name>` — restructure without behaviour change
    - `bugfix/<name>` — bug fix
    - `fix/<name>` — quick fix (e.g. config, docs, CI)
- **Tests**: co-located with source (`_test.go`). Cover happy path and at least one error path. Required for new control endpoints.
  - **Test-first for bug fixes**: write a failing test before touching production code.
- **Error format**: wrap errors as `fmt.Errorf("testserver: <context>: %w", err)`.
- **Dependency policy**: prefer stdlib; justify any new external dependency in the PR description.

## Critical Rules

1. **Read before write** — always read `main.go` and `README.md` fully before editing.
2. **Minimal changes** — one concern per edit; no drive-by refactors.
3. **No hardcoded secrets** — all configuration via environment variables.
4. **Goroutine lifecycle** — every goroutine launched must have an explicit exit condition. `close()` must not leak goroutines.
5. **Mutex discipline** — all `testServer` state mutations must be guarded by `ts.mu`. Never hold the mutex across blocking calls.
6. **Port stability** — `POST /restart` must rebind to the same WebSocket port so client URLs remain valid.
7. **READY protocol** — the `READY:<ws_port>:<control_port>` line on stderr is a contract consumed by client test harnesses. Do not change its format without updating all clients.
8. **STOP — test first, fix second** — when a bug is discovered, follow this exact sequence:
    1. Write a failing test that reproduces the bug.
    2. Confirm it fails.
    3. Fix the production code.
    4. Confirm it passes.
    5. Run `make check`.
9. **STOP — before every commit, verify this checklist:**
    1. Run `make check` and confirm it passes. Skip if the commit contains only non-code changes.
    2. Commit message follows [commit-message-instructions.md](instructions/commit-message-instructions.md).
    3. This commit contains exactly one logical change.
10. **Accuracy** — if you have questions, ask the user. Do not make assumptions.
11. **Language consistency** — when the user writes in Traditional Chinese, respond in Traditional Chinese; otherwise respond in English.

## Session Protocol

> Files under `doc/local/` are git-ignored and must **never** be committed.

- **At the start of every session**: check whether `doc/local/plan/` contains
  an in-progress plan for the current task, and read `doc/local/ai-learning.md`
  (if it exists) to recall past mistakes and techniques before writing any code.
- **Plan mode**: when implementing a new feature or multi-file fix, save a plan
  to `doc/local/plan/<feature-name>.md` before starting.
- **AI learning log**: at the end of a session where mistakes were made or
  reusable techniques were discovered, append a short entry to
  `doc/local/ai-learning.md`.
