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
  - **Pull request description**: must follow the repo's `.github/PULL_REQUEST_TEMPLATE.md`. Fill in every section (Summary, Changes, Checklist). Do not invent custom formats.
- **Tests**: co-located with source (`_test.go`). Cover happy path and at least one error path. Required for new control endpoints.
  - **Test-first for bug fixes**: write a failing test before touching production code.
- **Error format**: wrap errors as `fmt.Errorf("testserver: <context>: %w", err)`.
- **Dependency policy**: prefer stdlib; justify any new external dependency in the PR description.
- **File encoding**: all files must be UTF-8 without BOM. Do not use any other encoding.

## Feature Workflow

All new features and design changes follow this process — do not skip steps:

1. **Plan** — write idea to `doc/local/plan/<name>.md` (local only, git-ignored)
2. **Quick discussion** — feasibility + value check
3. **Go / No-go** — kill or proceed
4. **Layer check** — transport layer (wspulse implements) or application layer (write docs recipe instead)
5. **Issue** — repo-scoped work: open issue on this repo. Cross-repo/global work: open issue on [`wspulse/.github`](https://github.com/wspulse/.github). Include summary, scope, impact assessment, priority label + milestone
6. **Design discussion** — API surface, cross-SDK parity, contract/protocol updates, edge cases
7. **Task** — feature branch from `develop`, implement with tests, CHANGELOG entry, PR following template. **Repo-scoped**: link PR to the issue. **Global**: each PR mentions the global issue (e.g., `wspulse/.github#N`); after opening a PR, comment on the global issue with the PR link

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

## PR Comment Review — MANDATORY

When handling PR review comments, **every unresponded comment must be analyzed and responded to**. No comment may be silently ignored.

### 1. Fetch unresponded comments

Pull all comments that have not received a reply from the PR author. Bot-generated summaries (e.g. Copilot review overview) may be skipped; individual line comments from bots must still be evaluated.

### 2. Analyze each comment

Evaluate against:

| Criterion | Question |
|-----------|----------|
| **Validity** | Is the observation correct? Is the suggestion reasonable? |
| **Severity** | Is it a bug, a correctness issue, a design concern, or a style/preference nitpick? |
| **Cost** | What is the effort to address? Does the change introduce risk or scope creep? |

### 3. Present analysis for approval

Present all findings to the user before taking action. For each comment, show:
- The comment content and location
- Your assessment (validity, severity, cost)
- Your proposed decision (Fixed / Tracked / Won't fix / Not applicable) with reasoning

**Do not make any code changes or reply to comments until the user has reviewed and approved.** If there are disagreements, discuss until a consensus is reached.

### 4. Execute approved decisions

After approval, carry out each decision and respond on the PR:

- **`Fixed in {hash}. {what changed and why}`** — adopt and fix immediately. Bug and correctness issues must use this path unless the fix requires a separate PR due to scope.
- **`Tracked in TODOS.md — {reason for deferring}`** — adopt but defer. Add entry to repo root `TODOS.md` with context and PR comment link.
- **`Won't fix. {clear reasoning}`** — reject the suggestion with explanation.
- **`Not applicable — {explanation}`** — the comment does not apply (already handled, misunderstanding, duplicate, or already tracked in TODOS.md).

Duplicate or related comments may reference each other: `Same reasoning as {reference} above — {brief}`.

### 5. Zero unresponded comments before merge

The PR must have zero unaddressed comments before merge. This is a hard gate.

## Session Protocol

> Files under `doc/local/` are git-ignored and must **never** be committed.
> This includes plan files (`doc/local/plan/`), review records, and the AI learning log (`doc/local/ai-learning.md`).

### Start of every session — MANDATORY

**Do these steps before writing any code:**

1. Read `doc/local/ai-learning.md` **in full** to recall past mistakes. If the file is missing or empty, create it with the table header (see format below) before proceeding.
2. Check `doc/local/plan/` for any in-progress plan and read it fully.

### During feature work — doc before code

Before writing any production code, create or update `doc/local/plan/<feature-name>.md` with:

1. **What** — what are you changing or adding?
2. **Why** — what problem does it solve? What motivated this change?
3. **How** — what is the intended approach?

Keep it updated as the approach evolves. This is the primary cross-session context for understanding what was done and why.

For bug fixes, the failing test serves as the "what"; add a brief "why" and "how" to the plan file or `doc/local/ai-learning.md`.

### Review records

After conducting any review (code review, plan review, design review, PR review, etc.), record the findings for cross-session context:

- **Where to write**: this repo's `doc/local/`. If working in a multi-module workspace, also write to the workspace root's `doc/local/`.
- **Single truth**: write the full record in one location; the other location keeps a brief summary with a file path reference to the full record.
- **Acceptable formats**:
  1. Update the relevant plan file in `doc/local/plan/` with the review outcome.
  2. Dedicated review file in `doc/local/` if no relevant plan exists.
- **What to record**: review type, key findings, decisions made, action items, and resolution status.

### End of every session — MANDATORY

**Before closing the session, complete this checklist without exception:**

1. Append at least one entry to `doc/local/ai-learning.md` — **even if no mistakes were made**. Record what you confirmed, what technique worked, or what you observed. An empty file is a sign of non-compliance.
2. Update any in-progress plan in `doc/local/plan/` to reflect completed steps.
3. Verify `make check` passes in every module you edited.

**Entry format** for `doc/local/ai-learning.md`:

```
| Date       | Issue or Learning | Root Cause | Prevention Rule |
| ---------- | ----------------- | ---------- | --------------- |
| YYYY-MM-DD | <what happened or what you learned> | <why it happened> | <how to avoid it next time> |
```

**Writing to `ai-learning.md` is not optional. It is the primary cross-session improvement mechanism. An empty file proves the session protocol was ignored.**
