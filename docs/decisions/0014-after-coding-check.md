# 0014 — After coding: `make check` (gofmt, go vet, lint, go test), humans and AI

Status: built (2026-11-10)

## Decision
1. Every change to Go code ends with **`make check`**, run by whoever wrote it — a developer or an AI assistant.
   It runs, in order: **gofmt** (`gofmt -l` over the tracked Go files), **go vet**, **`make lint`** (staticcheck +
   golangci-lint, 0013), **`go test ./...`** (full suite, integration tests included).
2. Every step runs even when an earlier one fails; the target fails at the end if any did, so one run shows
   everything to fix.
3. Work is not done, committed or handed back until `make check` passes. `make check_short` (`go test -short`) is
   for iterating and never replaces the final run.
4. The rule is written where each reader looks: `README.md` for developers, `CLAUDE.md` (*After coding*) for the
   AI assistant, which also reports the result as it is — the failing step and its output, or that the
   integration tests couldn't run (no Docker).
5. Formatting is fixed with `make fmt`; the lint configs aren't loosened to get green without the maintainer's OK.

## Why
Each of these catches something the others don't (layout drift, suspicious constructs, dead code / wrong error
handling, broken behaviour). Running them only in a later review or a CI that the derivatives don't all have lets
findings pile up; running them at the end of every change keeps each service green, whoever wrote the change.

## Adopting it in a derivative
Needs 0013 first. Copy the Makefile `check` / `check_short` block (quote `"$(MAKE)"`: on Windows make lives under
`C:/Program Files`), the *After coding* section of `CLAUDE.md` and the `README.md` paragraph; run `make check`
until it passes.
