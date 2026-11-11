# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`go_boilerplate_vs` is a Go 1.26 HTTP API boilerplate (Gin + Postgres) organised as vertical feature slices.
It keeps one layering, error model and test conventions across the services built from it, and is
**fully JSON:API 1.1** on both sides (request documents, content negotiation, query parameters, errors) — see
*JSON:API*. The shipped example domain is `notes`; auth is local (bcrypt passwords + ES256 JWTs).

## Instruction files — read first

This file holds what applies everywhere. The topic sections live in **path-scoped rules** under `.claude/rules/`
(moved verbatim on 2026-11-04; the full original is in git history before that commit). Each rule loads by itself
when a file matching its `paths:` is read / edited, but **don't rely on that alone**:

- **Before designing or changing anything**, read (with the Read tool) every rule whose topic the task touches —
  the folders you edit **and** the cross-topic map below — and start each plan / design proposal with a line
  `Context read: <rule files>` so the user can see what was read.
- A "see *Section*" reference anywhere points to the section of that name; the index below says which file has it.
- **A new slice / folder** → add it to the `paths:` of every rule whose topic it touches, in the same change, and to
  the cross-topic map. A new section → into the rule of its topic (or a new rule, kept under 40,000 characters),
  never dropped.

Index:
- `.claude/rules/jsonapi.md` — Request flow (middleware order, handler pattern, error writing) and JSON:API 1.1
  (media type, request documents, query parameters, responses, errors). Sections: *Request flow*; *JSON:API*.
- `.claude/rules/logging.md` — `log/slog` conventions (`*Context` variants, `request_id` / `user_id`, detached
  goroutines, log assertions in tests). Sections: *Logging*.
- `.claude/rules/auth.md` — Register, login, token rotation, trusted / untrusted sessions, reuse detection,
  logout, rate limits and the client IP (trusted proxies). Sections: *Auth*; *Rate limits and the client IP*.
- `.claude/rules/slice-patterns.md` — reference code for the recurring slice patterns (owned resource + 404,
  create + read-back, lock-before-write tx over two queriers, partial PATCH, list filter/sort/page, tests,
  sqlc block). Kept even when no slice uses a pattern — see its *Maintenance rule*.

Cross-topic map (touching the left → also read the right):

- **A new slice / endpoint** → `jsonapi.md` (both sections) + `logging.md` + `slice-patterns.md`.
- **Deleting a slice** → `slice-patterns.md`: drop only its `Used by:` entries, never a pattern.
- **Middleware / router** (`internal/platform/middleware`, `server`) → `jsonapi.md` + `logging.md` (+ `auth.md`
  for `AccessTokenMiddleware` / `RequireRole`).
- **Users, roles, tokens, cookies, `refresh_tokens`, `auth_sessions`, `session/*`** → `auth.md` + `jsonapi.md`
  (*Request documents*).
- **A rate limit, a public endpoint, the client IP / `TRUSTED_PROXIES`** (`internal/platform/ratelimit`,
  `middleware/rate_limit.go`, `server.Security`) → `auth.md` (*Rate limits and the client IP*) + `jsonapi.md`
  (*Request flow*).
- **An error shape / status code** → `jsonapi.md` (*Errors*, *Request flow* step 4) + `logging.md`.
- **A goroutine / background worker, the activity log, an ops alert** → `logging.md`.
- **A migration** → *Gotchas* here + the rule of the table's feature (`users` / `refresh_tokens` → `auth.md`)
  + `slice-patterns.md` §1 for an owned, soft-deleted table.

## Decisions and derivatives

This boilerplate is the **spearhead** of the services derived from it. Architecture decisions are made, built and
tested here first; the derivatives follow. The log is `docs/decisions/` (numbered files, listed in its
`README.md`). **This repo is public:** the derivatives' names, paths, plans and the adoption tracker live only in
the gitignored `docs/decisions/DERIVATIVES.local.md` — never put them in a tracked file, nor where a piece of code
was ported from or that some service has (had) a gap. Only we, the first party, know the derivatives' weaknesses.

- **A global change** (anything a derivative would also want: security, auth, logging, JSON:API, tooling, test
  conventions, slice patterns) → a new decision file + a row in the local tracker, and **remind the user at
  the end to tell the derivatives** (they adopt one repo at a time, on the user's order). Purely local changes
  (the `notes` example domain, wording) don't need one.
- Building an *accepted* decision → set its status to built here, keep the tracker's derivative columns as they are.
- A derivative that adopts → its tracker cells change (and that repo's `CLAUDE.md` records it).

## After coding — `make check` (mandatory, humans and AI)

Every change to Go code ends with **`make check`**: gofmt → go vet → `make lint` (staticcheck + golangci-lint)
→ `go test ./...` (full suite, Docker). All four must pass before the work is called done, committed or handed
back. `make check_short` (unit tests only) is for iterating; it doesn't replace the final `make check`.
- A formatting finding → `make fmt`. A lint finding → fix it, or silence a false positive on its own line with
  the reason (see *Gotchas*). Never loosen `.golangci.yml` / `staticcheck.conf` to get green without the user's
  OK.
- Report the result as it is: which step failed and its output. If Docker isn't available, say the integration
  tests didn't run — don't present `check_short` as `check`.
- Decision 0014 (`docs/decisions/0014-after-coding-check.md`).

### A piece: done checklist, size, review stop (decision 0016)

- **Done, in this order:** `make check` green → `docs/API.md` updated if the API changed → every agreed
  behaviour has an outcome-asserting test, reported as a behaviour → test mapping → 100% coverage of new,
  non-generated code (`make test_coverage`; coverage supports, the mapping answers "is it safe?"). Only then
  `/code-review`.
- **One piece = one endpoint, one small feature or one complete bug fix**, with its tests; it builds and passes
  on its own and is its own commit (committed when the user orders it). Plan a ticket as a list of pieces up
  front. Not 5-line slivers (each review has a base cost), not the whole ticket as one diff.
- **Review stops by rule.** Fix now only: a bug in normal use (wrong result, wrong access, broken data), a
  security hole, a break of the `docs/API.md` contract. Everything else → the backlog
  (`docs/review-backlog.local.md`, gitignored — read it before a review so declined findings aren't argued
  again) without a long discussion. One round per piece; a second only to check the fixes of a fix-now finding.

## Commands

Tooling on PATH: `go`, `sqlc`, `migrate` (golang-migrate CLI), `mockgen` (github.com/golang/mock), `openssl`,
`staticcheck` + `golangci-lint` (`make lint_install`, pinned versions in the Makefile),
and a running Docker engine for integration tests (testcontainers). For `make sec` also `gosec`, `govulncheck`,
`gitleaks` (`make sec_install`, pinned versions in the Makefile) and `semgrep` (`pipx install semgrep`, or
`pip install --user semgrep` and put its Scripts / bin folder on PATH). `make` reads `.env` and exports it.

```sh
make run                       # go run cmd/api/main.go
docker compose up -d postgres  # local Postgres on DB_LOCAL_PORT
make migrate_up                # or migrate_up1 / migrate_down / migrate_down1 / migrate_dirty SEQ=<n>
make migrate_create NAME=add_something
make sqlc                      # regenerate every */sqlc package
make mock SOURCE=path/file.go DESTINATION=path/mock_file.go PKG_NAME=pkg
make ec_private && make ec_public
./build.sh && ./deploy.sh      # image go_boilerplate_vs_api:$(cat VERSION); deploy = graceful SIGTERM swap
make sec                       # gosec (medium+, no generated code), semgrep (p/golang + .semgrep/), govulncheck,
                               # gitleaks (git history); runs all four, fails at the end if any failed.
                               # Local only: `make sec SEC_PASSWORD=imsureitislocal`, typed on the command
                               # line (a value from .env / the environment is refused)

make check                     # AFTER CODING: gofmt, go vet, lint, go test ./... (full); fails if any step did
make check_short               # same with go test -short (no Docker) — for iterating, not the final run
make lint                      # staticcheck (staticcheck.conf) + golangci-lint (.golangci.yml)
make fmt                       # apply gofmt + goimports

go test ./... -short           # unit tests only (no Docker)
go test ./...                  # full suite; TestMain starts a migrated postgres:17 container per package
go test -run '^TestService_Update_I$' ./internal/features/note/update -count=1
```

The Postgres container start goes through `testhelper.startContainer`, which retries a failed start up to 3 times
(2 s, 4 s): the packages of one `go test ./...` share one Ryuk reaper and the first ones race to create it ("wait
for reaper … unexpected container status "created""). Ryuk stays on — it cleans up after a crash / Ctrl-C.

On Windows the Makefile sets `DOCKER_HOST=npipe:////./pipe/docker_engine` for the test targets. Set it yourself for a plain
`go test ./...` — without it testcontainers' auto-detection intermittently fails with "rootless Docker is not supported on
Windows".

## Architecture

### Package layout

- `cmd/api` — entrypoint: config → db pool → background group (`bgCtx`, `bg`) → validators → token maker → one
  service/handler per slice → `server.Handlers` → `server.NewServer(...).Run(ctx)` (graceful shutdown on
  SIGINT/SIGTERM) → `stopBg()` → `bg.Wait()` → the DB pool closes (see *Logging*).
- `internal/platform` — infrastructure: `config` (viper, `.env` optional), `logging` (`log/slog` setup: `New` /
  `Setup` / `Check` / `Fatal`, a handler adding `request_id`), `server` (`server.go` + `router.go` with the
  `Handlers` and `Security` structs), `middleware` (`RequestID`, `AccessLog`, `ActivityLog`, `CORS`, `OriginGuard`, `JSONAPIMediaType`,
  `RateLimitByIP`, `AccessTokenMiddleware`, `RequireRole`, its own sqlc for `GetAccessStatus` — user active + session live), `ratelimit`
  (token-bucket `Limiter` interface + in-memory `NewMemory` — see *Rate limits and the client IP*), `mailer`
  (`Mailer` interface: `NewSMTP`, the development `NewLog`, `Fake` for tests), `notifier` (ops alerts: `NewTelegram`
  / `NewNoop`), `jwttoken` (ES256), `db/tx` (generic
  transaction helper), `db/migration`, `constant`.
- `internal/shared` — cross-cutting: `apperror` (typed error → HTTP status, with a JSON:API `Source`),
  `jsonapi` (documents, serializer, sparse fieldsets, links, error objects, `?include` policy, the query
  parser), `handlerutil` (`ParseQuery` / `BindResource` / `ParamID` / `WriteData` / `WriteMeta` /
  `WriteError`), `validation` (`notblank`, `rfc3339`, `maxbytes`, `RegisterGinValidators`), `pagination`, `password`
  (bcrypt), `enum` (`Role`: `user` / `admin`), `requestid` (`Header`, `Key`, `With` / `FromContext` / `Detach`),
  `dto`, `constant` (context keys, cookie names), `onetimetoken` (email-link tokens: `New` → plain + SHA-256 hash),
  `utility` (`SafeGo` / `SafeGoContext`, `Group` + `GoFunc` for
  background work main waits for at shutdown, …), `testhelper`.
- `internal/features/<domain>/<action>` — the vertical slices:
  `auth/{register,login,refresh,logout,passwordresetrequest,passwordreset,passwordchange,emailverification,
  emailverificationrequest}`, `me`, `activitylog` (the recorder, no endpoint), `cleanup` (the worker, no endpoint),
  `note/{create,list,detail,update,delete}`, `session/{list,revoke}` (signed-in devices), `admin/user`. `<domain>/shared` holds what more than one slice of
  that domain needs (`note/shared`: the `NoteResource`, the include policy, the ownership helpers
  `GetOwnedNote` / `LockOwnedNote`, and the shared sqlc querier; `auth/shared`: cookies, reading the refresh
  token off a request, response shapes, the session revoke reasons, and the session queries — `StartSession`,
  `InsertRefreshToken`, `RevokeSession`, `RevokeUserSessions`).

### Slice structure

```
route.go        Route(rg *gin.RouterGroup, h H)   registers the endpoint(s)
handler.go      H interface + handler             ParseQuery → BindResource → service → WriteData
service.go      S interface + service             business logic; returns *apperror.AppError
store.go        StoreI + Store + Querier          only when the slice writes in a transaction
dto.go          QuerySpec, Request, Resource      Request = the document's attributes, gin binding +
                                                  `label` tags (shown in 422 details)
query/*.sql     sqlc queries                      the slice's own; sqlc/ is generated
mock_*.go       mockgen output                    MockH, MockS, MockStoreI + MockQuerier
main_test.go    TestMain → testhelper.RunWithDB
*_unit_test.go / *_integration_test.go
```

Rules:
- **One slice = one action** (or one small resource's CRUD, like `admin/user`). A flow that isn't the slice's
  own action gets its own slice.
- **A query needed by two slices goes in `<domain>/shared/query`, never copied into both.** A slice whose
  reads are all shared has no sqlc block (`note/detail`).
- A slice that writes in a transaction declares `Querier interface { sqlc.Querier; sharedsqlc.Querier }` in
  `store.go`; `RunInTx` binds both to one `*sql.Tx` (`txQueries` embeds both, the shared one under the alias
  `sharedQuerier`). `mockgen -source=store.go` emits `MockStoreI` + `MockQuerier`; unit tests make
  `RunInTx` call `fn(mockQuerier)` (see `slice-patterns.md` §3 and §7).
- Lock before you write: update/delete start with `LockNoteForUpdate` (`FOR UPDATE`) inside the tx.
- Not-yours reads as not-found: another user's note is a 404 with the same message as a missing one
  (`slice-patterns.md` §1).

### Adding a slice (checklist)

1. Migration (`make migrate_create NAME=...`) if the schema changes.
2. `query/queries.sql` + a block in `sqlc.yaml` (copy one) → `make sqlc` → `mockgen` the `sqlc/querier.go`.
3. `dto.go`, `service.go`, `handler.go`, `route.go` (+ `store.go` for a transaction) → `mockgen` them.
4. Add the handler to `server.Handlers`, one `Route` call in `NewRouter`, wire it in `cmd/api/main.go`, and a
   case in `server/router_unit_test.go`. The group: `authed` / `admin` for a signed-in caller; a public route goes
   into `guarded` (the general per-IP limit), or `limited` when it touches credentials or sends email — only one
   that runs automatically per tab (like refresh) goes straight into `public` (see `auth.md`).
5. Tests: handler unit (MockS), service unit (MockQuerier / MockStoreI), service integration (real Postgres).

### Config

`config.LoadConfig(".")` reads `.env` (optional) then OS env; `Validate()` requires every field except the
optional `REFRESH_TOKEN_DURATION` (default `168h`), `UNTRUSTED_SESSION_DURATION` (default `168h`), `COOKIE_SECURE` (default `true`), `COOKIE_DOMAIN`,
`LOG_FORMAT` (default `text`; `json` in production), `LOG_LEVEL` (default `info`) — those two must still
be values `logging.Check` accepts — and `TRUSTED_PROXIES` (comma-separated IPs / CIDRs, default none; each entry
is checked, see *Rate limits and the client IP*) and `ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]`,
default none; checked in `Validate`, stored normalised — lowercase, no trailing `/`; one list for CORS and the
origin guard, empty = both off with a startup warning). `APP_BASE_URL` (the front end, `http(s)://…`, no trailing
`/`; email links are built on it) is required; `SMTP_HOST`, `SMTP_FROM` (an address) are required except with
`ENVIRONMENT=development` (an empty `SMTP_HOST` there logs emails instead), `SMTP_PORT` defaults to `587`
(465 = implicit TLS, any other port must offer STARTTLS), `SMTP_USERNAME` / `SMTP_PASSWORD` optional.
`ACTIVITY_LOG_RETENTION` (optional, default `17520h` = 2 years, `0` = keep forever; the cleanup worker applies it).
`TELEGRAM_BOT_TOKEN` + `TELEGRAM_CHAT_ID` (optional, both or neither: ops alerts, see logging.md *Ops alerts*).
`REDIS_URL` (optional, `redis://` / `rediss://`, checked with `redis.ParseURL` without echoing it: the rate limits
in Redis, see *Gotchas*).
Durations are Go duration strings (`15m`).
See `.env.example`.

## Testing conventions

- `*_unit_test.go` → test names end `_U`; `*_integration_test.go` → `_I` and start with
  `if testing.Short() { t.Skip(...) }`.
- **Every test is table-driven** (`testCases` slice + loop with `t.Run`), `testify/require`, gomock.
- Integration tests use `testhelper.SeedUser` / `SeedNote`, `defer testhelper.Truncate(t, testDB)` per case,
  and read back with `testhelper.GetNote` / `GetUser`. Add new tables to `Truncate`.
- Mocks are committed; regenerate after changing an interface.

## Gotchas

- `.env` and `*.pem` are gitignored; never commit keys or secrets.
- `sqlc.yaml` has one block per slice with queries — a new one needs its entry, then `make sqlc`.
- `updated_at` is maintained by the `update_updated_at_column()` trigger on every table that has it.
- **`make sec` runs only on a developer's machine**, unlocked by `SEC_PASSWORD=imsureitislocal` typed on the
  command line (`$(origin SEC_PASSWORD)` must be `command line`, so a `.env` copied to a server can't carry it).
  `ENVIRONMENT` can't be the switch: laptops and the dev server both run `development`. A server gains nothing
  from the scan (same source) and would pay CPU, outbound calls and `.env` secrets exported to the tools. Never
  put `SEC_PASSWORD` in `.env` / `.env.example`.
- **Security scan (`make sec`) stays green.** A finding is fixed, or — when it's a false positive — silenced on
  its own line with the reason: `// #nosec G101 -- a cookie name, not a credential`,
  `// nosemgrep: <rule-id>` on the line before. Never a blanket exclude. Current ones: the cookie names and the
  login message (G101), the configurable `Secure` cookie (G124 + semgrep), pagination's bounded int32 offset
  (G115).
- **Lint (`make lint`) stays green**, same rule: fix, or silence on its own line with the reason —
  `//nolint:<linter> // why` (golangci-lint; `nolintlint` rejects one without a linter name or a reason),
  `//lint:ignore <check> why` on the line before (standalone staticcheck; a golangci `//nolint` doesn't reach it).
  staticcheck runs every check but ST1000. Current ones: logout returns nil for an unusable token (nilerr),
  refresh's `%v` on a possibly-nil err (errorlint), the plain string context key test (SA1029). Scoped config
  exclusions: noctx in tests / `testhelper`, testifylint `float-compare` (JSON numbers are float64).
- **Go files are LF** (`*.go text eol=lf` in `.gitattributes`): with `core.autocrlf=true` a CRLF checkout fails
  gofmt. Writing a file from Python on Windows emits CRLF — use `newline=''` or the editor tools.
- **Go version = go.mod `toolchain` + the Dockerfile builder image, bumped together** (`go1.26.10` /
  `golang:1.26.10-alpine`). `GOTOOLCHAIN=auto` makes the local `go` download and use the `toolchain` version for
  this module only; `build.sh` doesn't pull, so the image is pinned rather than a floating `1.26` tag (which
  reuses whatever is cached on the host). Most `govulncheck` hits are stdlib, fixed only by a newer patch release.
- **The container runs as uid 10001** (non-root, `USER` in the Dockerfile): files mounted into it (the EC keys)
  must be readable by that uid.
- **gin runs in release mode except when `ENVIRONMENT=development`** (`server.GinMode`, set in `main`): no route
  dump or gin debug output on a deployed server.
- **Rate limits are in memory unless `REDIS_URL` is set** (`internal/platform/ratelimit`): in memory they hold for
  one instance, so running several replicas multiplies every limit by the replica count — set `REDIS_URL`
  (Redis ≥ 7) first. With Redis down, the auth limits fall back to memory and the general per-IP limit fails open
  (one warning per limiter per 30 s); an unreachable Redis at start is only a warning.
  Behind a reverse proxy, `TRUSTED_PROXIES` must list it, or every client shares the proxy's IP.
- `.semgrep/` is `make sec`'s project ruleset; `semgrep --config .semgrep/` errors if the folder is missing, so
  keep at least one rule there.
- **Expired rows are pruned by `cleanup.Worker`** (decision 0005, in main's background group): a minute after
  start, then every 24 h, on every replica (idempotent deletes, 10,000 rows per batch): refresh tokens past
  their expiry or session cap (a revoked but unexpired one is **kept** — reuse detection needs it), sessions
  with no token left, expired one-time tokens, activity rows past `ACTIVITY_LOG_RETENTION`. One table's error is
  logged (`cleanup failed`, `table`) and the others still run; a run ends with `cleanup done` and the counts. A
  new table that grows → one more step there (one log field).
