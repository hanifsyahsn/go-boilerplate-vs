# go_boilerplate_vs

A Go 1.26 HTTP API boilerplate organised as **vertical slices**: every endpoint is a self-contained package
(route → handler → service → store → sqlc queries → tests). Gin, Postgres (sqlc + golang-migrate), ES256 JWT
auth, a **JSON:API 1.1** API (request and response documents, content negotiation, include / sparse
fieldsets / sort / pagination / filter), gomock + testcontainers tests.

It ships with a working example: auth (register / login / refresh / logout), `GET /api/me`, a `notes` CRUD
domain and an admin user directory — copy a slice to start a new one.

## Quick start

```sh
cp .env.example .env
make ec_private && make ec_public          # JWT signing keys (gitignored)
docker compose up -d postgres              # Postgres on localhost:5433
make migrate_up
make run                                   # http://localhost:8080
```

Tooling on PATH: `go`, `sqlc`, `migrate` (golang-migrate CLI), `mockgen` (`github.com/golang/mock`), `openssl`,
and Docker (for Postgres and the integration tests).

```sh
go test ./... -short     # unit tests only, no Docker
go test ./...            # + integration tests (testcontainers starts Postgres)
make sqlc                # regenerate */sqlc after editing query/*.sql or a migration
```

## Build & deploy

```sh
./build.sh     # docker compose build → go_boilerplate_vs_api:$(cat VERSION), prunes this project's dangling images
./deploy.sh    # SIGTERM the running container (30s grace, then stop), then `docker compose up -d app`
```

Bump `VERSION` before building; the version is baked in with `-ldflags -X main.Version`. Run the migrations
(`make migrate_up`) before deploying a release that adds one.

### Logging

Logs go to stdout through `log/slog`: `LOG_FORMAT=text` (default) for development, **`LOG_FORMAT=json` in
production** so a log aggregator can filter by field (`status`, `user_id`, …); `LOG_LEVEL` is `debug` / `info`
(default) / `warn` / `error`. Every request gets one `request` access-log record, and every line it writes
carries a `request_id` — the same value as the `X-Request-ID` response header, so `grep 3f2a9c1e` finds a whole
request — and, once the caller is authenticated, their `user_id`, so `grep user_id=42` finds everything one user
did.

The `request` record also has `method`, `path`, `status`, `latency_ms`, `client_ip` and `bytes`. A request the client abandoned (closed tab, navigated away) is logged at **warn**
with `cancelled=true` — and its error as `request cancelled` — instead of as a server error; a 5xx is `error`,
everything else `info`. A client error (4xx) logs its cause as `request rejected` (warn), a real failure as
`request failed` (error).

Behind nginx, forward nginx's own id so the proxy and app logs share it (the app keeps a well-formed incoming
`X-Request-ID`), and expose the header to a cross-origin frontend:

```nginx
location /api/ {
    proxy_set_header X-Request-ID $request_id;
    add_header Access-Control-Expose-Headers X-Request-ID always;   # only for a cross-origin client
    proxy_pass http://app:8080;
}
```

## Endpoints

| Method | Path | Auth | Slice |
|---|---|---|---|
| GET | `/healthz` | – | router (liveness) |
| POST | `/api/auth/register` | – | `auth/register` |
| POST | `/api/auth/login` | – | `auth/login` — sets the cookies, tokens also in `meta` |
| POST | `/api/auth/refresh` | refresh token (cookie or body) | `auth/refresh` — rotates: a new access **and** refresh token |
| POST | `/api/auth/logout` | – | `auth/logout` — revokes the login's refresh tokens, clears the cookies |
| GET | `/api/me` | user | `me` |
| POST | `/api/notes` | user | `note/create` |
| GET | `/api/notes` | user | `note/list` — `filter[q]`, `sort` (`title`, `created_at`, `updated_at`, `-` = desc), `page[number]`, `page[size]` |
| GET | `/api/notes/:id` | user (owner) | `note/detail` |
| PATCH | `/api/notes/:id` | user (owner) | `note/update` (partial) |
| DELETE | `/api/notes/:id` | user (owner) | `note/delete` (soft delete) |
| GET | `/api/admin/users` | admin | `admin/user` — `filter[q]`, `filter[is_active]`, `sort` (`name`, `email`, `created_at`), `page[...]` |
| PATCH | `/api/admin/users/:userId` | admin | `admin/user` — `is_active` |

Every request body is a JSON:API document sent as `application/vnd.api+json` (anything else is a 415):

```sh
curl -X POST localhost:8080/api/auth/login -H 'Content-Type: application/vnd.api+json' \
  -d '{"data":{"type":"sessions","attributes":{"email":"jane@example.com","password":"Passw0rd!"}}}'

curl -X PATCH localhost:8080/api/notes/5 -H 'Content-Type: application/vnd.api+json' -H "Authorization: Bearer $TOKEN" \
  -d '{"data":{"type":"notes","id":"5","attributes":{"title":"Renamed"}}}'
```

Register takes a `users` document, login / refresh / logout a `sessions` one. Every endpoint supports sparse
fieldsets (`?fields[notes]=title`), and note endpoints `?include=created_by` (the default; `?include=` for none).

Every response carries an `X-Request-ID` header; show it next to error messages (*"ref: …"*) so a user's report
leads straight to the logs.

Refresh tokens rotate on every refresh and live `REFRESH_TOKEN_DURATION` (default 7 days) from the last refresh,
so an active user stays logged in — on a device logged in with `"trust_device": true`. Without it the login
ends `UNTRUSTED_SESSION_DURATION` (default 7 days) after it, refresh or not. A client that doesn't use the cookies must store the new `refresh_token`
from `meta` each time: a spent one presented again revokes the whole login.

A new account is always a `user`;
promote the first admin by hand: `UPDATE users SET role = 'admin' WHERE email = '...';`

See [CLAUDE.md](CLAUDE.md) for the architecture and the conventions to follow when adding a slice.
