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
  logout. Sections: *Auth*.

Cross-topic map (touching the left → also read the right):

- **A new slice / endpoint** → `jsonapi.md` (both sections) + `logging.md`.
- **Middleware / router** (`internal/platform/middleware`, `server`) → `jsonapi.md` + `logging.md` (+ `auth.md`
  for `AccessTokenMiddleware` / `RequireRole`).
- **Users, roles, tokens, cookies, `refresh_tokens`** → `auth.md` + `jsonapi.md` (*Request documents*).
- **An error shape / status code** → `jsonapi.md` (*Errors*, *Request flow* step 4) + `logging.md`.
- **A goroutine / background worker** → `logging.md`.
- **A migration** → *Gotchas* here + the rule of the table's feature (`users` / `refresh_tokens` → `auth.md`).

## Commands

Tooling on PATH: `go`, `sqlc`, `migrate` (golang-migrate CLI), `mockgen` (github.com/golang/mock), `openssl`,
and a running Docker engine for integration tests (testcontainers). `make` reads `.env` and exports it.

```sh
make run                       # go run cmd/api/main.go
docker compose up -d postgres  # local Postgres on DB_LOCAL_PORT
make migrate_up                # or migrate_up1 / migrate_down / migrate_down1 / migrate_dirty SEQ=<n>
make migrate_create NAME=add_something
make sqlc                      # regenerate every */sqlc package
make mock SOURCE=path/file.go DESTINATION=path/mock_file.go PKG_NAME=pkg
make ec_private && make ec_public
./build.sh && ./deploy.sh      # image go_boilerplate_vs_api:$(cat VERSION); deploy = graceful SIGTERM swap

go test ./... -short           # unit tests only (no Docker)
go test ./...                  # full suite; TestMain starts a migrated postgres:17 container per package
go test -run '^TestService_Update_I$' ./internal/features/note/update -count=1
```

On Windows the Makefile sets `DOCKER_HOST=npipe:////./pipe/docker_engine` for the test targets. Set it yourself for a plain
`go test ./...` — without it testcontainers' auto-detection intermittently fails with "rootless Docker is not supported on
Windows".

## Architecture

### Package layout

- `cmd/api` — entrypoint: config → db pool → validators → token maker → one service/handler per slice →
  `server.Handlers` → `server.NewServer(...).Run(ctx)` (graceful shutdown on SIGINT/SIGTERM).
- `internal/platform` — infrastructure: `config` (viper, `.env` optional), `logging` (`log/slog` setup: `New` /
  `Setup` / `Check` / `Fatal`, a handler adding `request_id`), `server` (`server.go` + `router.go` with the
  `Handlers` struct), `middleware` (`RequestID`, `AccessLog`, `JSONAPIMediaType`, `AccessTokenMiddleware`,
  `RequireRole`, its own sqlc for `GetUserIsActive`), `jwttoken` (ES256), `db/tx` (generic transaction helper),
  `db/migration`, `constant`.
- `internal/shared` — cross-cutting: `apperror` (typed error → HTTP status, with a JSON:API `Source`),
  `jsonapi` (documents, serializer, sparse fieldsets, links, error objects, `?include` policy, the query
  parser), `handlerutil` (`ParseQuery` / `BindResource` / `ParamID` / `WriteData` / `WriteMeta` /
  `WriteError`), `validation` (`notblank`, `rfc3339`, `RegisterGinValidators`), `pagination`, `password`
  (bcrypt), `enum` (`Role`: `user` / `admin`), `requestid` (`Header`, `Key`, `With` / `FromContext` / `Detach`),
  `dto`, `constant` (context keys, cookie names), `utility` (`SafeGo` / `SafeGoContext`, …), `testhelper`.
- `internal/features/<domain>/<action>` — the vertical slices: `auth/{register,login,refresh,logout}`, `me`,
  `note/{create,list,detail,update,delete}`, `admin/user`. `<domain>/shared` holds what more than one slice of
  that domain needs (`note/shared`: the `NoteResource`, the include policy, the ownership helpers
  `GetOwnedNote` / `LockOwnedNote`, and the shared sqlc querier; `auth/shared`: cookies, reading the refresh
  token off a request, response shapes, and the `refresh_tokens` insert / family-revoke queries).

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
  `RunInTx` call `fn(mockQuerier)` (see `note/update/service_unit_test.go`).
- Lock before you write: update/delete start with `LockNoteForUpdate` (`FOR UPDATE`) inside the tx.
- Not-yours reads as not-found: another user's note is a 404 with the same message as a missing one.

### Adding a slice (checklist)

1. Migration (`make migrate_create NAME=...`) if the schema changes.
2. `query/queries.sql` + a block in `sqlc.yaml` (copy one) → `make sqlc` → `mockgen` the `sqlc/querier.go`.
3. `dto.go`, `service.go`, `handler.go`, `route.go` (+ `store.go` for a transaction) → `mockgen` them.
4. Add the handler to `server.Handlers`, one `Route` call in `NewRouter` (public / authed / admin group), wire
   it in `cmd/api/main.go`, and a case in `server/router_unit_test.go`.
5. Tests: handler unit (MockS), service unit (MockQuerier / MockStoreI), service integration (real Postgres).

### Config

`config.LoadConfig(".")` reads `.env` (optional) then OS env; `Validate()` requires every field except the
optional `REFRESH_TOKEN_DURATION` (default `168h`), `UNTRUSTED_SESSION_DURATION` (default `168h`), `COOKIE_SECURE` (default `true`), `COOKIE_DOMAIN`,
`LOG_FORMAT` (default `text`; `json` in production) and `LOG_LEVEL` (default `info`) — the last two must still
be values `logging.Check` accepts. Durations are Go duration strings (`15m`).
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
- `refresh_tokens` grows by one row per refresh and nothing prunes it yet; add a periodic
  `DELETE FROM refresh_tokens WHERE expires_at < now()` job when it matters.
