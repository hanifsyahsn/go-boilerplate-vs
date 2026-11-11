---
paths:
  - "internal/features/**/handler*.go"
  - "internal/features/**/route.go"
  - "internal/features/**/dto.go"
  - "internal/features/**/service.go"
  - "internal/shared/jsonapi/**"
  - "internal/shared/handlerutil/**"
  - "internal/shared/apperror/**"
  - "internal/shared/pagination/**"
  - "internal/shared/validation/**"
  - "internal/platform/middleware/**"
  - "internal/platform/server/**"
---
# Request flow and JSON:API

Moved verbatim from CLAUDE.md on 2026-11-04. Related rules: logging.md (how errors and requests are logged),
auth.md (`AccessTokenMiddleware`, the auth documents). What applies everywhere is in the root CLAUDE.md.

### Request flow

0. Every route (404 / 405 / panic included) runs `RequestID` → `AccessLog` → recovery → `CORS`; the engine trusts only
   `TRUSTED_PROXIES` for the client IP (see auth.md *Rate limits and the client IP*). `RequestID` keeps an
   incoming `X-Request-ID` matching `^[A-Za-z0-9._:-]{1,128}$`, else makes a UUID, and sets it on the response
   header (before the handlers), in the gin context (`requestid.Key`) and in the request context. `AccessLog`
   writes one `request` record (method, path, status, latency_ms, client_ip, bytes; `user_id` comes from the
   handler once auth has put the caller on the request context): warn with `cancelled=true` when the client went away (request context cancelled), else error
   for a 5xx, info for the rest.
   `CORS(ALLOWED_ORIGINS)` is on the engine (not `/api`) so a browser preflight, an `OPTIONS` no route declares,
   gets its 204 / 403 instead of the 405: a listed origin is echoed with credentials (never `*`), an unlisted
   one gets no CORS header (its preflight a 403 on `source.header` `Origin`); empty list = off. `Vary` is added
   to, never set, so `Origin` and `Accept` both stay.
1. `middleware.JSONAPIMediaType` (every `/api` route) checks content negotiation first (406 / 415). Every public
   route but refresh / logout then passes `middleware.RateLimitByIP` (the general per-IP budget), and register /
   login a second one (the auth budget) — 429 past either.
2. `AccessTokenMiddleware` reads the access token from the `app-access-token` cookie, else
   `Authorization: Bearer`, verifies it (ES256, `iss`, `aud` = `CLIENT_ID`, `exp`, `typ = access`), then
   re-reads `users.is_active` on **every** request (inactive / deleted → 401, DB error → 500). It sets
   `user_id` and `user_role` in the gin context, and the caller on the request context
   (`requestuser.With`) so logs written with `c.Request.Context()` carry `user_id`. `RequireRole(enum.RoleAdmin)` gates `/api/admin`.
3. The handler parses the query with `handlerutil.ParseQuery(c, type, spec)` **first** (before any side
   effect), binds the request document with `handlerutil.BindResource(c, ResourceSpec{Type, ID}, &req)`,
   calls the service, and writes with `handlerutil.WriteData` (or `WriteMeta` / `c.Status(204)`).
4. The service returns `*apperror.AppError` for anything the client should see; any other error becomes a
   500 in `handlerutil.WriteError`. `jsonapi.WriteErrorsContext(ctx, w, ...)` logs the wrapped cause with
   `status`, `code`, `err`: `request rejected` (warn, 4xx), `request failed` (error, 5xx), or `request
   cancelled` (warn) for a 5xx whose ctx is cancelled or whose chain holds `context.Canceled` — the ctx is the
   real signal, since lib/pq reports a cancelled query as a Postgres error. `context.DeadlineExceeded` stays an
   error (our timeout). `request_id` comes from ctx, else from the response header. `WriteErrors` is the
   `context.Background()` variant. `AppError.Unwrap()` returns `Err`, so `errors.Is` sees through it; the cause
   never reaches the response body.

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
  A limit that is really in bytes (bcrypt's 72, a column in bytes) uses the custom `maxbytes=N`, never `max=N`
  (which counts characters).
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
- **Error codes** (`apperror.Code`) carry their status and title: a domain declares its own in its package,
  `var CodeNoteArchived = apperror.NewCode("note_archived", http.StatusConflict)` (`.WithTitle("...")` only when
  the title isn't the status text), never in `internal/shared/apperror`. Read them with `Status()` / `Title()` /
  `String()`; the zero `Code{}` is 500. Decision 0015.
