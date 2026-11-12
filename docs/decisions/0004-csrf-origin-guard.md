# 0004 — CSRF `OriginGuard`

Status: built (2026-11-11).

## Decision
A state-changing request (not GET / HEAD / OPTIONS) that carries an `Origin` header must come from an allowed
origin, else 403. No `Origin` (curl, a server) passes. Allowlist from env, shared with 0009; empty = off.

## Why
The refresh token rides a cookie; SameSite=Lax alone doesn't close every CSRF path.

## As built
- `middleware.OriginGuard(allowedOrigins)` first on `/api` (before content negotiation), sharing
  `ALLOWED_ORIGINS` and its normalisation with CORS (0009). GET / HEAD / OPTIONS pass; any other method passes
  without an `Origin` header or with exactly one listed origin, and is a JSON:API 403 `forbidden` ("This request
  didn't come from an allowed origin.", `source.header` `Origin`) otherwise — `null`, an empty value and two
  `Origin` headers included. Empty list = off.
