---
paths:
  - "internal/features/auth/**"
  - "internal/features/me/**"
  - "internal/features/admin/**"
  - "internal/platform/middleware/access_token*"
  - "internal/platform/middleware/require_role*"
  - "internal/platform/jwttoken/**"
  - "internal/shared/password/**"
  - "internal/shared/enum/**"
  - "internal/platform/db/migration/000002_*"
  - "internal/platform/db/migration/000004_*"
  - "internal/platform/db/migration/000005_*"
---
# Auth: register, login, refresh, logout

Moved verbatim from CLAUDE.md on 2026-11-04. Related rules: jsonapi.md (request documents, the `sessions` /
`users` types), logging.md (`user_id` on the request context). What applies everywhere is in the root CLAUDE.md.

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
  coming back is never logged out; one who's away longer logs in again.
- **Trusted vs untrusted device** (migration 000005): login's optional `trust_device` attribute. Trusted →
  `refresh_tokens.session_expires_at` NULL, the session slides with no cap. Untrusted (the default) →
  `session_expires_at = login + UNTRUSTED_SESSION_DURATION` (optional, default 7 days), copied to every
  successor; refresh issues a token no longer than what's left and is a 401 past it.
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
