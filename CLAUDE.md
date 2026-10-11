# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`go_boilerplate_vs` is a Go 1.26 HTTP API boilerplate (Gin + Postgres) organised as vertical feature slices.
It was derived from the `memo-flow` service and keeps its layering, error model and test conventions, but is
**fully JSON:API 1.1** on both sides (request documents, content negotiation, query parameters, errors) — see
*JSON:API*. The shipped example domain is `notes`; auth is local (bcrypt passwords + ES256 JWTs).

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
- `internal/platform` — infrastructure: `config` (viper, `.env` optional), `server` (`server.go` + `router.go`
  with the `Handlers` struct), `middleware` (`JSONAPIMediaType`, `AccessTokenMiddleware`, `RequireRole`, its own
  sqlc for `GetUserIsActive`), `jwttoken` (ES256), `db/tx` (generic transaction helper), `db/migration`,
  `constant`.
- `internal/shared` — cross-cutting: `apperror` (typed error → HTTP status, with a JSON:API `Source`),
  `jsonapi` (documents, serializer, sparse fieldsets, links, error objects, `?include` policy, the query
  parser), `handlerutil` (`ParseQuery` / `BindResource` / `ParamID` / `WriteData` / `WriteMeta` /
  `WriteError`), `validation` (`notblank`, `rfc3339`, `RegisterGinValidators`), `pagination`, `password`
  (bcrypt), `enum` (`Role`: `user` / `admin`), `dto`, `constant` (context keys, cookie names), `utility`,
  `testhelper`.
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

### Request flow

1. `middleware.JSONAPIMediaType` (every `/api` route) checks content negotiation first (406 / 415).
2. `AccessTokenMiddleware` reads the access token from the `app-access-token` cookie, else
   `Authorization: Bearer`, verifies it (ES256, `iss`, `aud` = `CLIENT_ID`, `exp`, `typ = access`), then
   re-reads `users.is_active` on **every** request (inactive / deleted → 401, DB error → 500). It sets
   `user_id` and `user_role` in the gin context. `RequireRole(enum.RoleAdmin)` gates `/api/admin`.
3. The handler parses the query with `handlerutil.ParseQuery(c, type, spec)` **first** (before any side
   effect), binds the request document with `handlerutil.BindResource(c, ResourceSpec{Type, ID}, &req)`,
   calls the service, and writes with `handlerutil.WriteData` (or `WriteMeta` / `c.Status(204)`).
4. The service returns `*apperror.AppError` for anything the client should see; any other error becomes a
   500 in `handlerutil.WriteError`. `jsonapi.WriteErrors` logs 5xx at error level, the rest at warn.

### Auth

- Register takes a `users` document; role is always `user` (a `role` attribute is a 403); email unique
  ignoring case (`users_email_lower_key`, pq `23505` → 409). 201 with the account, no `Location` (a plain user
  has no URL of its own).
- Login takes a `sessions` document (`email`, `password`). One 401 message for unknown email and wrong
  password; "no longer active" only after the password matched. Sets `app-access-token` (path `/`) and
  `app-refresh-token` (path `/api/auth`) HttpOnly, SameSite=Lax cookies (`COOKIE_SECURE`, `COOKIE_DOMAIN`), and
  answers with the account as `data` and the tokens under `meta` for non-browser clients.
- Tokens carry `typ` (`access` / `refresh`); each verify rejects the other type.
- **Refresh tokens rotate** (migration 000004, table `refresh_tokens`, one row per refresh token keyed on its
  `jti`). Login starts a *family* (one per login / device). `POST /api/auth/refresh` locks the presented token's
  row, issues a **new pair** (both cookies replaced, both tokens in `meta`), inserts the successor in the same
  family and marks the old one `revoked_at` + `replaced_by`. The new refresh token lives
  `REFRESH_TOKEN_DURATION` (optional, default 7 days) from now, so it's an idle timeout: a user who keeps
  coming back is never logged out; one who's away longer logs in again. No absolute session cap.
- **Reuse detection**: a spent token presented again revokes its whole family (→ 401), since it was probably
  stolen. Exception: within `reuseGrace` (10s, `refresh/service.go`) of its rotation it's treated as two tabs
  refreshing at once — 401, family kept. Clients should still single-flight their refresh calls.
- Refresh re-reads the user, so the new access token carries the **current** role and a deactivated user gets
  none. An unknown jti (no row) is a 401.
- Refresh / logout take the token from the cookie, else an optional `sessions` document
  (`{"data":{"type":"sessions","attributes":{"refresh_token":"..."}}}`). Refresh answers with a meta-only
  document.
- Logout revokes the presented token's family and clears the cookies; a missing / invalid token is still a
  204. The access token is a stateless JWT and stays valid until it
  expires — keep `ACCESS_TOKEN_DURATION` short.

### JSON:API

The whole API is JSON:API 1.1; keep it that way in a new slice.

- **Media type** (`middleware.JSONAPIMediaType`, on `/api`): a request body must be
  `Content-Type: application/vnd.api+json` with no parameter but `profile` (else 415); an `Accept` that lists the
  JSON:API type only with other parameters is a 406. No extensions are supported. Every response has
  `Vary: Accept`. `/healthz` sits outside `/api`.
- **Request documents** (`handlerutil.BindResource` → `DecodeResource`): `{"data": {"type", "id"?,
  "attributes"}}`; the attributes decode into the slice's `Request` and are validated with gin binding tags.
  No `data` / not one object → 400; wrong `type` → 409; an `id` on a create (client-generated) → 403; on an
  update the `id` is required and must equal the URL's (else 409); any `relationships` → 403; an attribute the
  `Request` doesn't declare (read-only, unknown) → 403; a wrong JSON type → 422; a failed rule → 422 per field.
  Every error carries `source.pointer` (`/data/attributes/title`).
- **Updates are `PATCH` and partial**: `Request` fields are pointers, nil keeps the current value (`note/update`
  → `COALESCE(sqlc.narg(...), col)`; `admin/user` with nothing to change just reads the user back).
- **Query parameters** (`handlerutil.ParseQuery` with the endpoint's `jsonapi.QuerySpec`): `include` (an
  `IncludeConfig{Allowed, Default}`, opt-in with a server default — never hardcode a policy), `fields[TYPE]`
  (sparse fieldsets; `Fields` maps each type the response carries to its resource struct), `sort` (one field,
  `-` for descending, from `SortConfig.Allowed`; the SQL orders with a `CASE` per value), `page[number]` /
  `page[size]`, `filter[NAME]` (`Filters`). Whatever the spec leaves out is a 400 with `source.parameter`, and
  so is any other all-lowercase name (reserved by the spec); names with a non a-z character are
  implementation-specific and ignored. An endpoint with no parameters still calls `ParseQuery` with an empty
  spec (delete, refresh, logout).
- **Responses** (`jsonapi.WriteData` via `handlerutil.WriteData`): `jsonapi.version`, `data`, `included`
  (each related resource once, never one that's also primary data), `meta`, `links.self`; a
  `pagination.Meta` meta adds `first` / `last` / `prev` / `next` (`null` when unavailable). Struct tags drive
  serialization: `jsonapi:"id"`, `json:"name"` for an attribute, `jsonapi:"relation,<name>,<type>"` for a
  relationship (nil pointer → `data: null`, slice → to-many). A create is 201 with a `Location` header; a
  delete / logout 204 with no body.
- **Costly relationships** (to-many, big tables): `include` / `fields` only trim at serialization, so pass
  the `Query` (or two bools) to the service and load by need — `q.Includes("path")` → full rows;
  else `q.WantsField(type, "rel")` → ids only (linkage); else skip it. Never render an empty to-many to mean
  "not loaded". A cheap to-one (note → `created_by` via JOIN) just always loads.
- **Errors**: `{"jsonapi", "errors": [{status, code, title, detail, source}]}`; several errors take their
  shared status, else 400 (all 4xx) / 500. Unknown routes (404), a wrong method (405) and panics (500) are
  JSON:API errors too (`router.go`). A non-numeric path id is a 404 (it names no resource).

### Config

`config.LoadConfig(".")` reads `.env` (optional) then OS env; `Validate()` requires every field except the
optional `REFRESH_TOKEN_DURATION` (default `168h`), `COOKIE_SECURE` (default `true`) and `COOKIE_DOMAIN`. Durations are Go duration strings (`15m`).
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
