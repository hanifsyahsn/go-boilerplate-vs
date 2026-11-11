# go_boilerplate_vs API

HTTP API of the boilerplate: local accounts (bcrypt passwords, ES256 JWTs with rotating refresh
tokens), a personal **notes** CRUD, and a small admin user directory. The whole API speaks
**JSON:API 1.1** — request documents, content negotiation, query parameters and errors.

- **Base path:** `/api` (plus the unauthenticated `GET /healthz`)
- **Host:** value of `SERVER_ADDRESS` (e.g. `http://localhost:8080`)
- **Media type:** `application/vnd.api+json` for every request body and every response — see
  [Content negotiation](#content-negotiation).
- **Request id:** every response carries an `X-Request-ID` header — see [Request ids](#request-ids).

---

## Endpoints

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| `GET` | `/healthz` | — | Liveness probe + build version |
| `POST` | `/api/auth/register` | — | [Create an account](#post-apiauthregister) (role `user`) |
| `POST` | `/api/auth/login` | — | [Log in](#post-apiauthlogin): sets the auth cookies, returns the tokens |
| `POST` | `/api/auth/refresh` | refresh token | [Rotate the token pair](#post-apiauthrefresh) |
| `POST` | `/api/auth/logout` | refresh token | [Log out](#post-apiauthlogout): revoke the session, clear the cookies |
| `POST` | `/api/auth/password-reset-requests` | — | [Forgot password](#post-apiauthpassword-reset-requests): email a reset link |
| `POST` | `/api/auth/password-resets` | — | [Set a new password](#post-apiauthpassword-resets) from the link |
| `POST` | `/api/auth/email-verifications` | — | [Verify my email](#post-apiauthemail-verifications) from the link |
| `POST` | `/api/auth/password-changes` | user | [Change my password](#post-apiauthpassword-changes) |
| `GET` | `/api/me` | user | [The caller's own account](#get-apime) |
| `POST` | `/api/me/email-verification-requests` | user | [Send me a new verification link](#post-apimeemail-verification-requests) |
| `GET` | `/api/me/sessions` | user | [My signed-in devices](#get-apimesessions) |
| `DELETE` | `/api/me/sessions/{id}` | user | [Sign a device out](#delete-apimesessionsid) |
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

## Cross-origin requests (CORS)

A browser page on another origin may call the API only when its origin is listed in `ALLOWED_ORIGINS`
(`scheme://host[:port]`, compared ignoring case and a trailing `/`). Empty list = CORS is off: no CORS
header at all, so only same-origin pages can read the answers.

| Request | Answer |
| --- | --- |
| `Origin` listed | `Access-Control-Allow-Origin: <that origin>` (never `*`), `Access-Control-Allow-Credentials: true` (the auth cookies), `Access-Control-Expose-Headers: Location, Retry-After, X-Request-ID` |
| Preflight (`OPTIONS` + `Access-Control-Request-Method`) from a listed origin | **204** with `Access-Control-Allow-Methods: GET, POST, PATCH, DELETE`, `Access-Control-Allow-Headers: Accept, Authorization, Content-Type, X-Request-ID`, `Access-Control-Max-Age: 600` |
| Preflight from an unlisted origin | **403** `forbidden`, `detail` `This origin isn't allowed.`, `source.header` `Origin` |
| Any other request from an unlisted origin | no CORS header (the browser won't let the page read the answer) |

With the list set, every response carries `Vary: Origin` (next to `Vary: Accept` on `/api`).

**Cross-origin writes.** With the list set, a `POST` / `PATCH` / `DELETE` (any method but `GET`, `HEAD`,
`OPTIONS`) on `/api` that carries an `Origin` header must carry exactly one listed origin, else **403**
`forbidden`, `detail` `This request didn't come from an allowed origin.`, `source.header` `Origin` — checked
before anything else on `/api` (so before the 406 / 415). A request with no `Origin` (curl, a server, a mobile
app) passes; `Origin: null` doesn't.

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
the user **and the token's session** are **re-read from the database on every request**: a user
deactivated (or deleted), or a session signed out (logout, another device's sign-out) after the token
was issued, gets **401** on the very next call, not when the token expires.

| Failure | Status | `detail` |
| --- | --- | --- |
| No token, bad signature, expired, a refresh token used as an access token | 401 | `You're not authorized to perform this action.` |
| The user is inactive / gone | 401 | `Your account is no longer active.` |
| The token's session was signed out | 401 | `Your session has ended. Please sign in again.` |
| A plain user on `/api/admin/*` | 403 | `You don't have access to this.` |

→ on a `401` *"not authorized"*, [refresh](#post-apiauthrefresh) once and retry the call. On
*"no longer active"* or *"session has ended"*, drop the local state and go to the login screen.
An access token issued before sessions existed (no session claim) is a `401` *"not authorized"*;
its refresh fails too, so the user signs in again once.

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
| `sid` | string | the server's session id — internal, not a contract; don't build on it |

Ids in **responses are strings** (`"id": "42"`) while `sub` is a number — compare with `String(sub)`.

### How a session works

| Piece | Where it lives | Lifetime |
| --- | --- | --- |
| Access token (ES256 JWT) | cookie `app-access-token` (path `/`), also `meta.access_token` in the login / refresh body | `ACCESS_TOKEN_DURATION` (e.g. 15 min) |
| Refresh token (ES256 JWT, rotating) | cookie `app-refresh-token` (path `/api/auth`), also `meta.refresh_token` in the login / refresh body | `REFRESH_TOKEN_DURATION` **from its own issue** (default 7 days); on an untrusted device never past the session's end |

Both cookies are **HttpOnly**, `SameSite=Lax`, `Secure` unless `COOKIE_SECURE=false` (plain-HTTP
dev), scoped to `COOKIE_DOMAIN` when set. A browser client never reads them: it calls the API with
`credentials: "include"`.

- **Every login starts a session** (a refresh-token *family*): one per device / browser.
- **Refresh rotates**: each `POST /api/auth/refresh` spends the presented refresh token and returns a
  brand-new pair. The new refresh token lives `REFRESH_TOKEN_DURATION` from *now*, so it is an
  **idle timeout** — a user who keeps coming back is never logged out (on a trusted device, below);
  one who's away longer logs in again.
- **Trusted vs untrusted device**: a login with `"trust_device": true` slides as above with no cap.
  Without it (the default) the session ends `UNTRUSTED_SESSION_DURATION` (default 7 days) after the
  login, however often it refreshes: the last refresh token expires exactly then, and a refresh
  after it is a `401`.
- **Reuse detection**: presenting an **already-spent** refresh token revokes the **whole session**
  (it was probably stolen) → `401`, log in again. Exception: within **10 s** of its rotation it's
  treated as two tabs refreshing at once — `401`, but the session survives (the other tab already
  holds the new pair). **Single-flight** your refresh calls anyway.
- **Logout** revokes the session: its refresh tokens **and** its access tokens, at once (every request
  checks the session).

---

## Login, registration and sessions

All `/api/auth/*` endpoints are **public** (no access token).

**Rate limits** (in memory, per server instance; token buckets — the full allowance is available at once, then
it comes back evenly over the period):

| Limit | Applies to | Past it |
| --- | --- | --- |
| 300 requests / 1 min **per client IP** (IPv6: per /64) | every public route except refresh and logout (a flood guard; refresh and logout are never IP-limited, every open tab calls them) | `429` `too_many_requests` — `Too many attempts. Please wait a few minutes and try again.` — with a `Retry-After` header (seconds) |
| 300 requests / 15 min **per client IP** (IPv6: per /64) | register, login, forgot password, reset password and email verification together, on top of the one above | `429` `too_many_requests` — `Too many attempts. Please wait a few minutes and try again.` — with a `Retry-After` header (seconds) |
| 5 **failed** logins / 15 min **per email** (then one more every 3 min) | login: an unknown email and a wrong password both count; a successful login resets it | `429` `too_many_requests` — `Too many wrong attempts. Please wait a few minutes and try again.` — every login for that email, the right password too, until a try comes back |
| 100 **failed** logins / 6 h **per client IP**, across every email (IPv6: per /64; then one more every 3.6 min) | login: an unknown email and a wrong password both count; a successful login doesn't count (but doesn't clear the IP's earlier failures); a login refused for its email doesn't count either | `429` `too_many_requests` — `Too many wrong attempts. Please wait a few minutes and try again.` — every login from that IP, the right password too, until a try comes back |

Forgot password also sends at most **3 emails / 1 h per address**; past it the answer is the same `202` and
nothing is sent (so the endpoint can't flood an inbox).

The client IP is the connection's address; `X-Forwarded-For` / `X-Real-IP` count only from a proxy listed in
`TRUSTED_PROXIES`. On a 429, show the message and don't retry automatically.

### `POST /api/auth/register`

Creates an account. The role is always `user` — there is no way to register an admin.
**No `Location`** header: a plain user has no URL of its own (use [`GET /api/me`](#get-apime)).

**Request document** — `type` `users`, no `id`:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `name` | string | yes | not blank, ≤ 255; trimmed |
| `email` | string | yes | valid email, ≤ 255; trimmed; **unique ignoring case** |
| `password` | string | yes | at least 8 characters and at most **72 bytes** (bcrypt's input limit — see below) |

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
| 429 | `too_many_requests` | the IP used up its register + login budget (see *Rate limits* above) |
| 500 | `internal_error` | hashing / DB failure |

### `POST /api/auth/login`

**Request document** — `type` `sessions`:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `email` | string | yes | valid email, ≤ 255 |
| `password` | string | yes | ≤ 72 bytes (no longer password can be registered) |
| `trust_device` | boolean | no (default `false`) | `true` ("remember this device"): the session slides with no cap; otherwise it ends `UNTRUSTED_SESSION_DURATION` after the login |

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
| 429 | `too_many_requests` | the email or the IP used up its failed logins, or the IP its register + login budget (see *Rate limits* above) |
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

Revokes the presented refresh token's whole session (its refresh and access tokens) and clears both cookies. The token comes from
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

### `POST /api/auth/password-reset-requests`

"Forgot password": emails a link to `APP_BASE_URL/reset-password?token=…` (valid **1 hour**; asking again makes
the earlier links stop working). **Always `202`** with the same body — an unknown email, a deactivated account
and the per-address limit look exactly like a sent link, so the endpoint never tells which emails have an
account. The email goes out in the background.

```bash
curl -s -X POST "$BASE/api/auth/password-reset-requests" -H "$CT" \
  -d '{"data":{"type":"password-reset-requests","attributes":{"email":"jane@example.com"}}}'
```

```json
{
  "jsonapi": { "version": "1.1" },
  "meta": { "message": "If an account exists for that email, we've sent a link to reset the password." },
  "links": { "self": "/api/auth/password-reset-requests" }
}
```

| Status | `code` | When |
| --- | --- | --- |
| 422 | `invalid_request_body` | `email` missing or not an email |
| 429 | `too_many_requests` | the IP's budget (see *Rate limits*) |

### `POST /api/auth/password-resets`

Sets a new password with the link's `token` (the front end reads it from `?token=`). The link works **once**.
Every session of the account ends (every device signs in again), the email counts as verified (the link proved
it), and the owner gets a *"Your password was changed"* email. **`204`**, no body.

```bash
curl -s -X POST "$BASE/api/auth/password-resets" -H "$CT" \
  -d '{"data":{"type":"password-resets","attributes":{"token":"<from the link>","password":"N3w-Passw0rd"}}}'
```

| Status | `code` | When |
| --- | --- | --- |
| 422 | `invalid_token` | `This link is invalid or has expired. Ask for a new one.` — `source.pointer` `/data/attributes/token`: unknown, already used or expired |
| 422 | `invalid_request_body` | `password` under 8 characters or over 72 bytes, `token` missing or over 64 characters |
| 429 | `too_many_requests` | the IP's budget (see *Rate limits*) |

```json
{
  "jsonapi": { "version": "1.1" },
  "errors": [
    {
      "status": "422", "code": "invalid_token", "title": "Unprocessable Entity",
      "detail": "This link is invalid or has expired. Ask for a new one.",
      "source": { "pointer": "/data/attributes/token" }
    }
  ]
}
```

### `POST /api/auth/password-changes`

Changes the caller's password (an access token is required). Every **other** device is signed out at once; this
one stays signed in. The owner gets a *"Your password was changed"* email. **`204`**, no body.

```bash
curl -s -X POST "$BASE/api/auth/password-changes" -H "$CT" -H "Authorization: Bearer $TOKEN" \
  -d '{"data":{"type":"password-changes","attributes":{"current_password":"Passw0rd!","password":"N3w-Passw0rd"}}}'
```

| Status | `code` | When |
| --- | --- | --- |
| 422 | `wrong_password` | `The current password is incorrect.` — `source.pointer` `/data/attributes/current_password` |
| 422 | `invalid_request_body` | a password missing, the new one under 8 characters, either over 72 bytes |
| 429 | `too_many_requests` | `Too many wrong attempts. Please wait a few minutes and try again.` — 5 wrong current passwords / 15 min per account (then one more every 3 min), the right one too until a try comes back |

### `POST /api/auth/email-verifications`

Registering emails a link to `APP_BASE_URL/verify-email?token=…` (valid **24 hours**). The front end posts its
`token` here; the account's email then counts as verified (`email_verified` in [`GET /api/me`](#get-apime)).
An unverified email doesn't block login. The link works **once**. **`204`**, no body.

```bash
curl -s -X POST "$BASE/api/auth/email-verifications" -H "$CT" \
  -d '{"data":{"type":"email-verifications","attributes":{"token":"<from the link>"}}}'
```

| Status | `code` | When |
| --- | --- | --- |
| 422 | `invalid_token` | `This link is invalid or has expired. Ask for a new one.` — `source.pointer` `/data/attributes/token` |
| 422 | `invalid_request_body` | `token` missing or over 64 characters |

### `POST /api/me/email-verification-requests`

Emails the caller a new verification link (the earlier ones stop working). No body is needed; an empty
`{"data":{"type":"email-verification-requests"}}` is accepted. **`202`**:

```json
{
  "jsonapi": { "version": "1.1" },
  "meta": { "message": "We've sent a new verification link." },
  "links": { "self": "/api/me/email-verification-requests" }
}
```

| Status | `code` | When |
| --- | --- | --- |
| 409 | `email_already_verified` | `Your email is already verified.` |
| 429 | `too_many_requests` | `Too many attempts. Please wait a few minutes and try again.` — 3 links / 1 h per account |

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

### `GET /api/me/sessions`

The caller's signed-in devices: one `sessions` resource per **live** login (not signed out, and still able to
refresh). The `id` is the session id to sign it out with.

| Attribute | Type | Meaning |
| --- | --- | --- |
| `user_agent` | string | the browser / app of the last login or refresh |
| `client_ip` | string | its IP at that time |
| `trusted` | bool | logged in with `trust_device` |
| `created_at` / `last_used_at` | RFC 3339 | the login / the last refresh |
| `expires_at` | RFC 3339 or `null` | when an untrusted session ends; `null` = trusted, no cap |
| `current` | bool | the session of this request's own access token |

Query: `sort` = `last_used_at` / `created_at` (`-` for descending; default `-last_used_at`),
`fields[sessions]`. No include, filter or pagination (`400`).

```bash
curl -s "$BASE/api/me/sessions" -H "Authorization: Bearer $TOKEN"
```

```json
{
  "jsonapi": { "version": "1.1" },
  "data": [
    {
      "type": "sessions", "id": "6a1f0c9e-3b52-4d8e-9a10-5c2e7f4b8d31",
      "attributes": {
        "user_agent": "Mozilla/5.0 …", "client_ip": "203.0.113.7", "trusted": false,
        "created_at": "2026-11-11T08:00:00Z", "last_used_at": "2026-11-11T09:30:00Z",
        "expires_at": "2026-11-18T08:00:00Z", "current": true
      }
    }
  ],
  "links": { "self": "/api/me/sessions" }
}
```

### `DELETE /api/me/sessions/{id}`

Signs one of the caller's devices out: its refresh token stops working and its access token is a `401`
*"Your session has ended. Please sign in again."* on its next call. **`204`**, no body. Signing out the
**current** session is a logout: the auth cookies are cleared too.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | `That session doesn't exist.` — not a UUID, someone else's, or already signed out / expired |

```bash
curl -s -X DELETE "$BASE/api/me/sessions/6a1f0c9e-3b52-4d8e-9a10-5c2e7f4b8d31" -H "Authorization: Bearer $TOKEN"
```

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

### Request ids

**Every response** — success, error, `404` / `405`, even a crash — carries an `X-Request-ID` header,
and every server log line that request writes has the same `request_id`. A well-formed id the client
(or a proxy) sends in `X-Request-ID` is kept (letters, digits and `._:-`, at most 128 characters);
anything else is replaced with a new UUID.

- **Show it next to error messages**, e.g. *"We couldn't save that note. (ref: 3f2a9c1e…)"*. When a
  user reports it, one search of the logs finds everything that request did.
- A **cross-origin** browser client can only read the header if the server (or the proxy in front of
  it) sends `Access-Control-Expose-Headers: X-Request-ID`.

```bash
curl -i "$BASE/api/notes/999" -H "Authorization: Bearer $TOKEN"
# HTTP/1.1 404 Not Found
# X-Request-ID: 3f2a9c1e-7b4d-4e0a-9f1b-2c3d4e5f6a7b
```

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

**Password length is in bytes.** bcrypt reads at most 72 bytes. Plain letters, digits and symbols are one byte
each, an accented letter (`é`) two and an emoji four — so 40 accented letters are 40 characters but 80 bytes,
and are refused with `422` "Password is too long (at most 72 bytes). Accented letters and emoji count as more
than one character, so use a shorter one." (`source.pointer` `/data/attributes/password`), on register and on
login. A form can count `new TextEncoder().encode(pw).length` to warn early.

The `detail` names the field by its **label**, not its JSON name: `title` → "Title", `body` →
"Content", `name` → "Name", `email` → "Email", `password` → "Password", `refresh_token` →
"Refresh token", `trust_device` → "Trust device", `is_active` → "Active flag".

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
| `page[number]` / `page[size]` | 1-based page / size. Clamped, never rejected: number < 1 → 1, number > 21474837 → 21474837 (any page past the data is empty), size < 1 → 20, size > 100 → 100 |
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
| `too_many_requests` | 429 | Register / login rate limit (per IP, or failed logins per email / per IP) |
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
7. On any error, show `errors[].detail` with the response's `X-Request-ID` as a *"ref: …"* the user
   can quote to support ([Request ids](#request-ids)).

### A non-browser client (mobile, service)

Same flow, but store `meta.access_token` / `meta.refresh_token` from login, send
`Authorization: Bearer <access_token>`, and refresh with the token in a `sessions` document. After
every refresh **replace both** stored tokens — the old refresh token is spent, and presenting it
again later logs the session out.

### Current limitations

- No way to change a user's role over the API.
- `refresh_tokens` grows by one row per refresh; nothing prunes it yet.
