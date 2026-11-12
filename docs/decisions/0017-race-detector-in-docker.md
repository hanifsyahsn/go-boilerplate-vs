# 0017 — Race detector: `go test -race` inside Docker (`make test_race`)

Status: built (2026-11-12) — `make test_race` (`RACE_IMAGE`, `PACKAGE` to narrow)

## Decision
1. Data races are checked with Go's race detector (`go test -race`), and it **always runs inside a Docker
   container**, never with the developer's local Go: `make test_race`.
2. The container is the Debian `golang` image at the **same Go patch version as `go.mod`'s `toolchain`**
   (today `golang:1.26.11`, the Makefile `RACE_IMAGE`), bumped together with it and the Dockerfile builder image (see *Gotchas* in
   `CLAUDE.md`). Debian, not alpine: it ships gcc, which `-race` needs (cgo).
3. The repo is mounted into the container; the Go module and build caches live in named Docker volumes, so only
   the first run downloads modules.
4. It runs the **unit tests** (`go test -race -short ./...`). The integration tests start Postgres through
   testcontainers, which would need the Docker socket inside the container; they stay out until that is set up
   on purpose.
5. **When:** a change that touches concurrency must pass `make test_race` before it's called done: goroutines,
   channels, `sync` / `sync/atomic`, state shared between requests (limiters, caches, recorders, workers), or a
   test that runs things in parallel. It's not part of `make check` (0014): `-race` is several times slower and
   most changes don't need it. The result is reported like `make check`'s — the race report as it is.
6. A data race found is a bug to fix now (decision 0016's "fix now": wrong result in normal use), never silenced.

## Why
`-race` only works with cgo on amd64 / arm64. A developer machine may not have that — a 32-bit Go install
(windows/386) or no gcc rules it out entirely — so "run `-race` locally" silently doesn't happen. Docker is
already required for the integration tests, so a container gives every developer and the AI assistant the same
working race detector, on the architecture production runs (linux/amd64).

Outcome tests under concurrency (exact counts from N parallel calls) catch many races but not reliably — a race
may not show in a given run. The race detector reports the unsynchronised access itself, whether the run
happened to go wrong or not.

## Adopting it in a derivative
Copy the Makefile `test_race` target (image pinned to the repo's `toolchain` patch version, the two cache
volumes), the *After coding* note in `CLAUDE.md` and the `README.md` line; run `make test_race` once and fix
whatever it reports.
