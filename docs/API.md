# go_boilerplate_vs API

HTTP API of the boilerplate: local accounts (bcrypt passwords, ES256 JWTs with rotating refresh
tokens), a personal **notes** CRUD, and a small admin user directory. The whole API speaks
**JSON:API 1.1** — request documents, content negotiation, query parameters and errors.

- **Base path:** `/api` (plus the unauthenticated `GET /healthz`)
- **Host:** value of `SERVER_ADDRESS` (e.g. `http://localhost:8080`)
- **Media type:** `application/vnd.api+json` for every request body and every response — see
  [Content negotiation](#content-negotiation).

---

## Endpoints

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| `GET` | `/healthz` | — | Liveness probe + build version |
| `POST` | `/api/auth/register` | — | [Create an account](#post-apiauthregister) (role `user`) |
| `POST` | `/api/auth/login` | — | [Log in](#post-apiauthlogin): sets the auth cookies, returns the tokens |
| `POST` | `/api/auth/refresh` | refresh token | [Rotate the token pair](#post-apiauthrefresh) |
| `POST` | `/api/auth/logout` | refresh token | [Log out](#post-apiauthlogout): revoke the session, clear the cookies |
| `GET` | `/api/me` | user | [The caller's own account](#get-apime) |
| `POST` | `/api/notes` | user | [Create a note](#1-create-note) |
| `GET` | `/api/notes` | user | [List my notes](#2-list-notes) (paginated, searchable, sortable) |
| `GET` | `/api/notes/{id}` | user | [Get one of my notes](#3-get-note) |
| `PATCH` | `/api/notes/{id}` | user | [Partially update one of my notes](#4-update-note) |
| `DELETE` | `/api/notes/{id}` | user | [Soft-delete one of my notes](#5-delete-note) |
| `GET` | `/api/admin/users` | admin | [User directory](#6-list-users-admin) |
| `PATCH` | `/api/admin/users/{userId}` | admin | [Activate / deactivate a user](#7-update-user-admin) |

---

## Running the `curl` examples

Every endpoint below has a copy-pasteable `curl`. They use three shell variables — set them once:

```bash
export BASE="http://localhost:8080"          # or your SERVER_ADDRESS
export CT="Content-Type: application/vnd.api+json"
export TOKEN="<meta.access_token from POST /api/auth/login>"   # no "Bearer " prefix
```

Instead of `-H "Authorization: Bearer $TOKEN"` a browser-like client may keep the cookies:
`curl -c jar.txt -b jar.txt …` on login, then `-b jar.txt` everywhere else.

---

## Content negotiation

Checked first on every `/api` route, before authentication.

| Rule | Otherwise |
| --- | --- |
| A request **with a body** sends `Content-Type: application/vnd.api+json`, with no media type parameter except `profile` | **415** `unsupported_media_type` |
| `Accept`, when it lists `application/vnd.api+json`, must list it at least once **without** parameters other than `profile` (`Accept: */*`, `application/json` or no `Accept` at all are fine) | **406** `not_acceptable` |
| No JSON:API extensions (`ext=…`) are supported | 415 / 406 |

Every response carries `Vary: Accept`. `/healthz` sits outside `/api` and skips these checks.

```json
{
  "jsonapi": { "version": "1.1" },
  "errors": [
    {
      "status": "415", "code": "unsupported_media_type", "title": "Unsupported Media Type",
      "detail": "Send the request body as application/vnd.api+json.",
      "source": { "header": "Content-Type" }
    }
  ]
}
```

---

## Authentication

Every endpoint except `/healthz` and `/api/auth/*` sits behind the access-token middleware. The
access token comes from [logging in](#post-apiauthlogin) (or [refreshing](#post-apiauthrefresh))
and is sent one of two ways:

| Method | How |
| --- | --- |
| Cookie | `app-access-token=<jwt>` — set by login / refresh, a browser sends it on its own |
| Header | `Authorization: Bearer <jwt>` |

The cookie is checked first; when it is absent the `Authorization` header is used.

The token is verified (ES256 signature, `iss`, `aud` = `CLIENT_ID`, `exp`, `typ` = `access`), then
the user is **re-read from the database on every request**: a user deactivated (or deleted) after
the token was issued gets **401** on the very next call, not when the token expires.

| Failure | Status | `detail` |
| --- | --- | --- |
| No token, bad signature, expired, a refresh token used as an access token | 401 | `You're not authorized to perform this action.` |
| The user is inactive / gone | 401 | `Your account is no longer active.` |
| A plain user on `/api/admin/*` | 403 | `You don't have access to this.` |

→ on a `401` *"not authorized"*, [refresh](#post-apiauthrefresh) once and retry the call. On
*"no longer active"*, drop the local state and go to the login screen.

### Who is the caller?

The access token's payload (decode the JWT; the server verifies it, the client doesn't need to):

| Claim | Type | Use it for |
| --- | --- | --- |
| `sub` | number | the caller's **user id** |
| `name` / `email` | string | display |
| `role` | string | `user` / `admin` ([Enums](#enums)) — **a hint only**: a snapshot from when the token was issued. The server authorizes `/api/admin` on it, but a refresh re-reads the role, so drive the admin menu from `role` in [`GET /api/me`](#get-apime) (live) |
| `typ` | string | always `access` (a refresh token says `refresh`) |
| `exp` / `iat` | number | unix seconds; after `exp` every call is `401` → refresh |
| `iss` / `aud` / `jti` | string | issuer, `CLIENT_ID`, token id |

Ids in **responses are strings** (`"id": "42"`) while `sub` is a number — compare with `String(sub)`.

### How a session works

| Piece | Where it lives | Lifetime |
| --- | --- | --- |
| Access token (ES256 JWT) | cookie `app-access-token` (path `/`), also `meta.access_token` in the login / refresh body | `ACCESS_TOKEN_DURATION` (e.g. 15 min) |
| Refresh token (ES256 JWT, rotating) | cookie `app-refresh-token` (path `/api/auth`), also `meta.refresh_token` in the login / refresh body | `REFRESH_TOKEN_DURATION` **from its own issue** (default 7 days) |

Both cookies are **HttpOnly**, `SameSite=Lax`, `Secure` unless `COOKIE_SECURE=false` (plain-HTTP
dev), scoped to `COOKIE_DOMAIN` when set. A browser client never reads them: it calls the API with
`credentials: "include"`.

- **Every login starts a session** (a refresh-token *family*): one per device / browser.
- **Refresh rotates**: each `POST /api/auth/refresh` spends the presented refresh token and returns a
  brand-new pair. The new refresh token lives `REFRESH_TOKEN_DURATION` from *now*, so it is an
  **idle timeout** — a user who keeps coming back is never logged out; one who's away longer logs in
  again.
- **Reuse detection**: presenting an **already-spent** refresh token revokes the **whole session**
  (it was probably stolen) → `401`, log in again. Exception: within **10 s** of its rotation it's
  treated as two tabs refreshing at once — `401`, but the session survives (the other tab already
  holds the new pair). **Single-flight** your refresh calls anyway.
- **Logout** revokes the session's refresh tokens. The **access token stays valid until it expires**
  (it's a stateless JWT) — that's why `ACCESS_TOKEN_DURATION` is short.

---

## Login, registration and sessions

All `/api/auth/*` endpoints are **public** (no access token).

### `POST /api/auth/register`

Creates an account. The role is always `user` — there is no way to register an admin.
**No `Location`** header: a plain user has no URL of its own (use [`GET /api/me`](#get-apime)).

**Request document** — `type` `users`, no `id`:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `name` | string | yes | not blank, ≤ 255; trimmed |
| `email` | string | yes | valid email, ≤ 255; trimmed; **unique ignoring case** |
| `password` | string | yes | 8–72 characters (72 = bcrypt's input limit) |

Any other attribute (`role`, `is_active`, …) is a **403**.

```bash
curl -X POST "$BASE/api/auth/register" -H "$CT" -d '{
  "data": {
    "type": "users",
    "attributes": { "name": "Jane Doe", "email": "jane@example.com", "password": "Passw0rd!" }
  }
}'
```

**Response — `201 Created`**

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "users", "id": "1",
    "attributes": { "email": "jane@example.com", "name": "Jane Doe", "role": "user" }
  },
  "links": { "self": "/api/auth/register" }
}
```

Supports `fields[users]`.

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 409 | `conflict` | `That email is already registered.` (any case: `JANE@example.com` = `jane@example.com`) |
| 403 | `forbidden` | an attribute that isn't writable — `"role" can't be set on this endpoint.` |
| 422 | `unprocessable_entity` | a field failed validation — one error per field (see [Validation error](#validation-error)) |
| 500 | `internal_error` | hashing / DB failure |

### `POST /api/auth/login`

**Request document** — `type` `sessions`:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `email` | string | yes | valid email, ≤ 255 |
| `password` | string | yes | ≤ 72 |

```bash
curl -X POST "$BASE/api/auth/login" -H "$CT" -c jar.txt -d '{
  "data": {
    "type": "sessions",
    "attributes": { "email": "jane@example.com", "password": "Passw0rd!" }
  }
}'
```

**Response — `200 OK`** — sets both cookies, answers with the **account** as `data` and the
**tokens** under `meta` (for clients that don't use cookies: mobile, server-to-server):

```
Set-Cookie: app-access-token=eyJhbGciOi…; Path=/; Max-Age=899; HttpOnly; SameSite=Lax
Set-Cookie: app-refresh-token=eyJhbGciOi…; Path=/api/auth; Max-Age=604799; HttpOnly; SameSite=Lax
```

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "users", "id": "1",
    "attributes": { "email": "jane@example.com", "name": "Jane Doe", "role": "user" }
  },
  "meta": {
    "access_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9…",
    "access_token_expires_at": "2026-10-09T17:27:59.2966735+07:00",
    "refresh_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9…",
    "refresh_token_expires_at": "2026-10-16T17:12:59.2976996+07:00"
  },
  "links": { "self": "/api/auth/login" }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 401 | `unauthorized` | `The email or password is incorrect.` — the **same** message for an unknown email and a wrong password, so an account's existence never leaks |
| 401 | `unauthorized` | `Your account is no longer active.` — only **after** the password matched |
| 422 | `unprocessable_entity` | a field failed validation |
| 500 | `internal_error` | token signing / DB failure |

### `POST /api/auth/refresh`

Rotates the pair: spends the presented refresh token, issues a **new access + refresh token**
(both cookies replaced, both tokens in `meta`). The user is re-read, so the new access token carries
the **current** role and a deactivated user gets none.

The refresh token is taken from the `app-refresh-token` cookie; **without the cookie**, send it in
an optional `sessions` document:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `refresh_token` | string | no (when the cookie is sent) | ≤ 4096 |

```bash
# browser-style (cookie jar from the login)
curl -X POST "$BASE/api/auth/refresh" -b jar.txt -c jar.txt

# token in the body
curl -X POST "$BASE/api/auth/refresh" -H "$CT" -d '{
  "data": { "type": "sessions", "attributes": { "refresh_token": "<meta.refresh_token>" } }
}'
```

**Response — `200 OK`** — a **meta-only** document; store **both** new tokens, the old refresh
token is spent:

```json
{
  "jsonapi": { "version": "1.1" },
  "meta": {
    "access_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9…",
    "access_token_expires_at": "2026-10-09T17:27:59.4334119+07:00",
    "refresh_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9…",
    "refresh_token_expires_at": "2026-10-16T17:12:59.4339876+07:00"
  },
  "links": { "self": "/api/auth/refresh" }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 401 | `unauthorized` | `Your session has expired. Please log in again.` — no / invalid / expired token, an access token sent instead, an unknown token, a revoked session, or a **reused** spent token (which also revokes the session — see [How a session works](#how-a-session-works)) |
| 401 | `unauthorized` | `Your account is no longer active.` |
| 500 | `internal_error` | DB / token signing failure |

→ on any `401` here: drop the local state, go to the login screen. **Never retry a refresh in a loop.**

### `POST /api/auth/logout`

Revokes the presented refresh token's whole session and clears both cookies. The token comes from
the cookie or the same optional `sessions` document as [refresh](#post-apiauthrefresh). A missing
or invalid token is **still a `204`** — logging out never fails from the client's point of view.

```bash
curl -X POST "$BASE/api/auth/logout" -b jar.txt -c jar.txt
```

**Response — `204 No Content`** (no body)

```
Set-Cookie: app-access-token=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax
Set-Cookie: app-refresh-token=; Path=/api/auth; Max-Age=0; HttpOnly; SameSite=Lax
```

**Errors:** a malformed `sessions` document (`400` / `409` / `422`, see [Request documents](#request-documents)); `500` (DB failure while revoking).

### `GET /api/me`

Any authenticated user. The caller's own account, read **live** from the database — `role` here may
be newer than the token's `role` claim.

```bash
curl -X GET "$BASE/api/me" -H "Authorization: Bearer $TOKEN"
```

**Response — `200 OK`**

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "users", "id": "1",
    "attributes": {
      "created_at": "2026-10-09T10:12:58.940147Z",
      "email": "jane@example.com",
      "name": "Jane Doe",
      "role": "user"
    }
  },
  "links": { "self": "/api/me" }
}
```

**Errors:** `401` (no / bad token, inactive account), `404` `Your account was not found.`, `500`.

---

## Response conventions

### Success

Every body has `jsonapi.version` and `links.self`; the payload is `data` (one resource or an array),
plus `included` and `meta` when there are any.

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "notes", "id": "1",
    "attributes": { "title": "Groceries", "body": "milk, eggs", "created_at": "…", "updated_at": "…" },
    "relationships": { "created_by": { "data": { "type": "users", "id": "1" } } }
  },
  "included": [
    { "type": "users", "id": "1", "attributes": { "email": "jane@example.com", "name": "Jane Doe" } }
  ],
  "links": { "self": "/api/notes/1" }
}
```

- **Ids are strings.**
- **Timestamps** are RFC 3339; resource timestamps are UTC (`2026-10-09T10:12:59.318351Z`).
- **`included`** carries each related resource **once**, and never one that's also primary data.
- A **create** is `201` with a `Location` header; a **delete** / **logout** is `204` with no body.
- A response with no primary data (refresh, `/healthz`) is **meta-only**.

### Error

```json
{
  "jsonapi": { "version": "1.1" },
  "errors": [
    {
      "status": "404",
      "code": "not_found",
      "title": "Not Found",
      "detail": "We couldn't find that note. It may have been removed."
    }
  ]
}
```

Every `detail` is **user-safe text** (never a stack trace or SQL) — show it as-is. When the error
is about one part of the request, `source` says which — **exactly one** of:

| `source` member | Points at | Example |
| --- | --- | --- |
| `pointer` | a member of the request document (JSON Pointer) | `/data/attributes/title`, `/data/id` |
| `parameter` | a query parameter | `sort`, `filter[is_active]` |
| `header` | a request header | `Content-Type`, `Accept` |

Several errors share the HTTP status when they have the same one; otherwise the response is `400`
(all `4xx`) or `500`.

### Validation error

A failed rule is **422 Unprocessable Entity**, one error per field, each with its `source.pointer`
(so the form can put the message under the right input):

```json
{
  "jsonapi": { "version": "1.1" },
  "errors": [
    { "status": "422", "code": "unprocessable_entity", "title": "Unprocessable Entity",
      "detail": "Name cannot be blank.", "source": { "pointer": "/data/attributes/name" } },
    { "status": "422", "code": "unprocessable_entity", "title": "Unprocessable Entity",
      "detail": "Email must be a valid email address.", "source": { "pointer": "/data/attributes/email" } },
    { "status": "422", "code": "unprocessable_entity", "title": "Unprocessable Entity",
      "detail": "Password must be at least 8 characters long.", "source": { "pointer": "/data/attributes/password" } }
  ]
}
```

The `detail` names the field by its **label**, not its JSON name: `title` → "Title", `body` →
"Content", `name` → "Name", `email` → "Email", `password` → "Password", `refresh_token` →
"Refresh token", `is_active` → "Active flag".

### Request documents

Every body is a JSON:API document with **one resource object** under `data`:

```json
{ "data": { "type": "notes", "id": "1", "attributes": { "title": "…" } } }
```

| Problem | Status | `detail` / `source.pointer` |
| --- | --- | --- |
| not JSON | 400 | `The request body is not valid JSON.` |
| no `data`, or `data` isn't one object | 400 | `The request body must be a JSON:API document with a data member.` — `/data` |
| wrong `type` | 409 | `This endpoint takes "notes" resources.` — `/data/type` |
| an `id` on a create | 403 | `Client-generated ids aren't supported.` — `/data/id` |
| an update without `id`, or an `id` other than the URL's | 409 | `The id in the body doesn't match the URL.` — `/data/id` |
| any `relationships` | 403 | `Relationships can't be set on this endpoint.` — `/data/relationships` |
| an attribute the endpoint doesn't take (read-only / unknown) | 403 | `"role" can't be set on this endpoint.` — `/data/attributes/role` |
| an attribute of the wrong JSON type (`"title": 5`) | 422 | `/data/attributes/title` |
| a failed rule | 422 | one per field — see [Validation error](#validation-error) |

### Query parameters

Each endpoint accepts only what its section lists. Anything else **reserved by JSON:API** (an
all-lowercase name such as `foo` or `page`) is a **400** with `source.parameter`; a name with any
other character (`_t`, `cacheBust`) is implementation-specific and **ignored**.

| Parameter | Meaning |
| --- | --- |
| `include` | comma list of relationships to expand into `included`. Each endpoint has a **default** (notes: `created_by`); `include=` (empty) expands nothing |
| `fields[TYPE]` | sparse fieldsets: comma list of the attributes / relationships of `TYPE` to return, e.g. `fields[notes]=title&fields[users]=name` |
| `sort` | **one** field, `-` prefix for descending, e.g. `-created_at` |
| `page[number]` / `page[size]` | 1-based page / size. Clamped, never rejected: number < 1 → 1, size < 1 → 20, size > 100 → 100 |
| `filter[NAME]` | the endpoint's filters |

A paginated list answers with `meta` and four pagination links (`null` when there's no such page):

```json
"meta": { "page": 1, "page_size": 1, "total": 2, "total_page": 2 },
"links": {
  "self":  "/api/notes?page[size]=1",
  "first": "/api/notes?page%5Bnumber%5D=1&page%5Bsize%5D=1",
  "last":  "/api/notes?page%5Bnumber%5D=2&page%5Bsize%5D=1",
  "prev":  null,
  "next":  "/api/notes?page%5Bnumber%5D=2&page%5Bsize%5D=1"
}
```

The links keep the request's other parameters (filters, sort, fields) — follow them as-is.

### Error code reference

| `code` | HTTP | Meaning |
| --- | --- | --- |
| `bad_request` | 400 | Malformed body / unsupported or invalid query parameter |
| `unauthorized` | 401 | Missing / invalid / expired token, inactive account, wrong credentials, expired session |
| `forbidden` | 403 | Not an admin on `/api/admin`; a client-generated id, a relationship or a read-only attribute in the body |
| `not_found` | 404 | Unknown route; a missing / deleted / **someone else's** note; a non-numeric path id |
| `method_not_allowed` | 405 | e.g. `PUT /api/notes/1` (updates are `PATCH`) |
| `not_acceptable` | 406 | See [Content negotiation](#content-negotiation) |
| `conflict` | 409 | Email already registered; wrong resource `type`; body `id` ≠ URL id; an admin deactivating themselves |
| `unsupported_media_type` | 415 | See [Content negotiation](#content-negotiation) |
| `unprocessable_entity` | 422 | A field failed validation / has the wrong JSON type |
| `internal_error` | 500 | Unexpected server error (`Something went wrong, please try again.`) — also a panic |

---

## Enums

| Field | Value | Description |
| --- | --- | --- |
| `role` | `user` | default — notes only; what [register](#post-apiauthregister) always creates |
| `role` | `admin` | may also call `/api/admin/*`. Granted directly in the database (no endpoint) |

---

## Notes

A note belongs to the user who created it. **Not-yours reads as not-found**: another user's note
answers exactly like a missing or deleted one (`404`, same message), so its existence never leaks.
Deleting is a **soft delete** — the row stays, the note disappears from every endpoint.

### The `notes` resource

| Member | Type | Description |
| --- | --- | --- |
| `id` | string | Note primary key |
| `title` | string | ≤ 255, not blank |
| `body` | string | ≤ 10 000, may be `""` |
| `created_at` / `updated_at` | string | RFC 3339, UTC; `updated_at` moves on every update |
| `relationships.created_by` | → `users` | the creator (always the caller) |

`included` `users` resource: `name`, `email`.

Every note endpoint supports `include` (allowed: `created_by`, **default** `created_by`) and
`fields[notes]` / `fields[users]`.

### 1. Create note

```
POST /api/notes
```

**Request document** — `type` `notes`, no `id`:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `title` | string | yes | not blank, ≤ 255; trimmed |
| `body` | string | no | ≤ 10 000; omitted ⇒ `""` |

```bash
curl -X POST "$BASE/api/notes" -H "Authorization: Bearer $TOKEN" -H "$CT" -d '{
  "data": { "type": "notes", "attributes": { "title": "Groceries", "body": "milk, eggs" } }
}'
```

**Response — `201 Created`**, `Location: /api/notes/1`

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "notes", "id": "1",
    "attributes": {
      "body": "milk, eggs",
      "created_at": "2026-10-09T10:12:59.318351Z",
      "title": "Groceries",
      "updated_at": "2026-10-09T10:12:59.318351Z"
    },
    "relationships": { "created_by": { "data": { "type": "users", "id": "1" } } }
  },
  "included": [
    { "type": "users", "id": "1", "attributes": { "email": "jane@example.com", "name": "Jane Doe" } }
  ],
  "links": { "self": "/api/notes" }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 403 | `forbidden` | an `id` in the body (`Client-generated ids aren't supported.`), a `relationships` member, an unknown attribute |
| 409 | `conflict` | `type` isn't `notes` |
| 422 | `unprocessable_entity` | `Title is required.` / `Title cannot be blank.` / `Title must be at most 255 characters long.` / `Content must be at most 10000 characters long.` |
| 401 / 500 | | |

### 2. List notes

```
GET /api/notes
```

The caller's live notes.

**Query parameters** — all optional:

| Param | Rules | Default |
| --- | --- | --- |
| `filter[q]` | case-insensitive substring of `title` **or** `body` | — |
| `sort` | one of `created_at`, `updated_at`, `title`, each optionally `-`-prefixed | `-created_at` (newest first) |
| `page[number]` / `page[size]` | see [Query parameters](#query-parameters) | `1` / `20` (max `100`) |
| `include` | `created_by` | `created_by` |
| `fields[notes]` / `fields[users]` | sparse fieldsets | all |

```bash
# default — newest first, creator included
curl -g "$BASE/api/notes" -H "Authorization: Bearer $TOKEN"

# search "egg", A→Z by title, titles only, no included
curl -g "$BASE/api/notes?filter[q]=egg&sort=title&fields[notes]=title&include=" -H "Authorization: Bearer $TOKEN"

# second page of 10, oldest first
curl -g "$BASE/api/notes?sort=created_at&page[number]=2&page[size]=10" -H "Authorization: Bearer $TOKEN"
```

(`-g` stops curl from treating `[` `]` as a URL glob.)

**Response — `200 OK`** (`?page[size]=1`)

```json
{
  "jsonapi": { "version": "1.1" },
  "data": [
    {
      "type": "notes", "id": "2",
      "attributes": {
        "body": "",
        "created_at": "2026-10-09T10:12:59.330201Z",
        "title": "Ideas",
        "updated_at": "2026-10-09T10:12:59.330201Z"
      },
      "relationships": { "created_by": { "data": { "type": "users", "id": "1" } } }
    }
  ],
  "included": [
    { "type": "users", "id": "1", "attributes": { "email": "jane@example.com", "name": "Jane Doe" } }
  ],
  "meta": { "page": 1, "page_size": 1, "total": 2, "total_page": 2 },
  "links": {
    "self": "/api/notes?page[size]=1",
    "first": "/api/notes?page%5Bnumber%5D=1&page%5Bsize%5D=1",
    "last": "/api/notes?page%5Bnumber%5D=2&page%5Bsize%5D=1",
    "prev": null,
    "next": "/api/notes?page%5Bnumber%5D=2&page%5Bsize%5D=1"
  }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 400 | `bad_request` | `Can't sort by "body". Allowed values: created_at, updated_at, title.` (`source.parameter` `sort`); `Sort by one field at a time.`; `Can't include "x". Allowed values: created_by.`; `fields[x]` for a type the response doesn't carry; any other lowercase parameter — `"foo" is not a supported query parameter.` |
| 401 / 500 | | |

### 3. Get note

```
GET /api/notes/{id}
```

```bash
curl -g "$BASE/api/notes/1" -H "Authorization: Bearer $TOKEN"

# title + creator name only
curl -g "$BASE/api/notes/1?fields[notes]=title,created_by&fields[users]=name" -H "Authorization: Bearer $TOKEN"
```

**Response — `200 OK`** — the [`notes` resource](#the-notes-resource), same shape as
[create](#1-create-note), `links.self` = `/api/notes/1`. With the sparse fieldsets above:

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "notes", "id": "1",
    "attributes": { "title": "Groceries" },
    "relationships": { "created_by": { "data": { "type": "users", "id": "1" } } }
  },
  "included": [ { "type": "users", "id": "1", "attributes": { "name": "Jane Doe" } } ],
  "links": { "self": "/api/notes/1?fields[notes]=title,created_by&fields[users]=name" }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | missing, deleted or someone else's note — `We couldn't find that note. It may have been removed.`; a non-numeric id (`/api/notes/abc`) — `That note was not found.` |
| 400 | `bad_request` | an unsupported query parameter |
| 401 / 500 | | |

### 4. Update note

```
PATCH /api/notes/{id}
```

**Partial**: an attribute left out keeps its current value. The note is locked (`FOR UPDATE`)
for the update, then read back.

**Request document** — `type` `notes`, `id` **required** and equal to the URL's:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `title` | string | no | not blank, ≤ 255; trimmed |
| `body` | string | no | ≤ 10 000; `""` clears it |

```bash
curl -X PATCH "$BASE/api/notes/1" -H "Authorization: Bearer $TOKEN" -H "$CT" -d '{
  "data": { "type": "notes", "id": "1", "attributes": { "title": "Groceries (week 2)" } }
}'
```

**Response — `200 OK`** — the updated note (`body` unchanged, `updated_at` moved):

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "notes", "id": "1",
    "attributes": {
      "body": "milk, eggs",
      "created_at": "2026-10-09T10:12:59.318351Z",
      "title": "Groceries (week 2)",
      "updated_at": "2026-10-09T10:12:59.388563Z"
    },
    "relationships": { "created_by": { "data": { "type": "users", "id": "1" } } }
  },
  "included": [
    { "type": "users", "id": "1", "attributes": { "email": "jane@example.com", "name": "Jane Doe" } }
  ],
  "links": { "self": "/api/notes/1" }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | missing, deleted or someone else's note (nothing is changed) |
| 409 | `conflict` | no `id`, or `The id in the body doesn't match the URL.`; `type` isn't `notes` |
| 403 | `forbidden` | a `relationships` member, an unknown attribute |
| 422 | `unprocessable_entity` | `Title cannot be blank.`, length limits, a wrong JSON type |
| 405 | `method_not_allowed` | `PUT` instead of `PATCH` |
| 401 / 500 | | |

### 5. Delete note

```
DELETE /api/notes/{id}
```

Soft-deletes the note (`deleted_at` / `deleted_by` are set). Takes no query parameters.

```bash
curl -X DELETE "$BASE/api/notes/2" -H "Authorization: Bearer $TOKEN"
```

**Response — `204 No Content`** (no body)

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | missing, **already deleted**, or someone else's note — deleting twice is a `404` the second time |
| 401 / 500 | | |

---

## Administration

Every `/api/admin/*` endpoint requires an **admin** access token (`role` = `admin`); a plain user
gets `403` `You don't have access to this.`

### The `users` resource (admin)

| Attribute | Type | Description |
| --- | --- | --- |
| `id` | string | User primary key |
| `name` / `email` | string | |
| `role` | string | `user` / `admin` ([Enums](#enums)) |
| `is_active` | boolean | `false` ⇒ can't log in or refresh, and every access token they hold is rejected on its next call |
| `created_at` / `updated_at` | string | RFC 3339, UTC |

Supports `fields[users]`.

### 6. List users (admin)

```
GET /api/admin/users
```

Every user, active or not.

| Param | Rules | Default |
| --- | --- | --- |
| `filter[q]` | case-insensitive substring of `name` **or** `email` | — |
| `filter[is_active]` | `true` / `false` (also `1` / `0`); anything else → 400 | — (both) |
| `sort` | one of `created_at`, `name`, `email`, each optionally `-`-prefixed | `-created_at` |
| `page[number]` / `page[size]` | see [Query parameters](#query-parameters) | `1` / `20` (max `100`) |
| `fields[users]` | sparse fieldset | all |

```bash
curl -g "$BASE/api/admin/users?filter[is_active]=true&sort=name&page[size]=1" -H "Authorization: Bearer $TOKEN"
```

**Response — `200 OK`**

```json
{
  "jsonapi": { "version": "1.1" },
  "data": [
    {
      "type": "users", "id": "3",
      "attributes": {
        "created_at": "2026-10-09T10:12:59.444668Z",
        "email": "admin@example.com",
        "is_active": true,
        "name": "Admin",
        "role": "admin",
        "updated_at": "2026-10-09T10:12:59.444668Z"
      }
    }
  ],
  "meta": { "page": 1, "page_size": 1, "total": 2, "total_page": 2 },
  "links": {
    "self": "/api/admin/users?page[size]=1&filter[is_active]=true&sort=name",
    "first": "/api/admin/users?filter%5Bis_active%5D=true&page%5Bnumber%5D=1&page%5Bsize%5D=1&sort=name",
    "last": "/api/admin/users?filter%5Bis_active%5D=true&page%5Bnumber%5D=2&page%5Bsize%5D=1&sort=name",
    "prev": null,
    "next": "/api/admin/users?filter%5Bis_active%5D=true&page%5Bnumber%5D=2&page%5Bsize%5D=1&sort=name"
  }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 400 | `bad_request` | `filter[is_active] must be true or false.` (`source.parameter` `filter[is_active]`); a bad `sort`; an unsupported parameter |
| 403 | `forbidden` | not an admin |
| 401 / 500 | | |

### 7. Update user (admin)

```
PATCH /api/admin/users/{userId}
```

Activates or deactivates a user. Only `is_active` is writable — `name`, `email` and `role` are a
`403`. A document with no attributes changes nothing and just returns the user.

**Request document** — `type` `users`, `id` required and equal to the URL's:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `is_active` | boolean | no | an admin can't set **their own** to `false` |

```bash
curl -X PATCH "$BASE/api/admin/users/1" -H "Authorization: Bearer $TOKEN" -H "$CT" -d '{
  "data": { "type": "users", "id": "1", "attributes": { "is_active": false } }
}'
```

**Response — `200 OK`**

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "users", "id": "1",
    "attributes": {
      "created_at": "2026-10-09T10:12:58.940147Z",
      "email": "jane@example.com",
      "is_active": false,
      "name": "Jane Doe",
      "role": "user",
      "updated_at": "2026-10-09T10:12:59.477105Z"
    }
  },
  "links": { "self": "/api/admin/users/1" }
}
```

The deactivated user is cut off on their **next request** (`401` `Your account is no longer
active.`), and their refresh tokens stop working.

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | `That user was not found.` (also a non-numeric `userId`) |
| 409 | `conflict` | `You can't deactivate your own account.`; body `id` ≠ URL id; `type` isn't `users` |
| 403 | `forbidden` | not an admin; a non-writable attribute (`"role" can't be set on this endpoint.`) |
| 422 | `unprocessable_entity` | `is_active` isn't a boolean |
| 401 / 500 | | |

---

## Health

### `GET /healthz`

Unauthenticated liveness probe, outside `/api` (no content negotiation). `version` is the build's
`main.Version`, baked in from the `VERSION` file by `build.sh` (`dev` under `make run`).

```bash
curl "$BASE/healthz"
```

**Response — `200 OK`**

```json
{
  "jsonapi": { "version": "1.1" },
  "meta": { "status": "up", "version": "1.0.0" },
  "links": { "self": "/healthz" }
}
```

---

## Client guide

### A typical browser session

1. `POST /api/auth/login` with `credentials: "include"` → cookies are set; keep `data` (the account).
2. `GET /api/me` for the live role → show / hide the admin menu.
3. Call the API with `credentials: "include"` — no `Authorization` header needed.
4. On a `401` *"You're not authorized to perform this action."*: call `POST /api/auth/refresh`
   **once** (single-flight across tabs), then retry the original call.
5. Refresh `401` or *"Your account is no longer active."* → go to the login screen.
6. `POST /api/auth/logout` on sign-out.

### A non-browser client (mobile, service)

Same flow, but store `meta.access_token` / `meta.refresh_token` from login, send
`Authorization: Bearer <access_token>`, and refresh with the token in a `sessions` document. After
every refresh **replace both** stored tokens — the old refresh token is spent, and presenting it
again later logs the session out.

### Current limitations

- No password reset / change, no email verification, no way to change a user's role over the API.
- Logout doesn't revoke the access token (it lives until `exp`).
- `refresh_tokens` grows by one row per refresh; nothing prunes it yet.
