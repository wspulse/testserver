# Contributing to wspulse/testserver

Thank you for your interest in contributing. This document describes the process and conventions expected for all contributions.

## Before You Start

- Open an issue to discuss significant changes before starting work.
- Read the [testserver plan](../.github/doc/plan/testserver-plan.md) for architecture context.

## Development Setup

```bash
git clone https://github.com/wspulse/testserver
cd testserver
# Clone core and server alongside testserver (required for local development)
git clone https://github.com/wspulse/core ../core
git clone https://github.com/wspulse/server ../server
go mod tidy
```

Requires: Go 1.26+, [golangci-lint](https://golangci-lint.run/), [goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports).

## Pre-Commit Checklist

Run `make check` before every commit. It runs in order:

1. `make fmt` — formats all source files
2. `make lint` — runs `go vet` and `golangci-lint`; must pass with zero warnings
3. `make test` — runs tests with `-race`; must pass

If any step fails, do not commit.

## Commit Messages

Follow the format in [`.github/instructions/commit-message-instructions.md`](.github/instructions/commit-message-instructions.md):

```
<type>: <subject>

1.<reason> → <change>
```

All commit messages must be in English.

## Naming Conventions

- Use full words for all identifiers. Cryptic abbreviations are not acceptable.
- Allowed short forms: ID, URL, HTTP, API, JSON, Msg, Err, Ctx, Buf, Cfg.
