# 0006 — Signed-in devices and remote sign-out

Status: built (2026-11-11).

## Decision
The caller can list the devices (sessions) they're signed in on and sign any of them out; signing out the
current one is a logout (cookies cleared). Builds on the trusted / untrusted sessions.

## Why
The natural continuation of the session model; needed by any service with its own login.

## As built (part 1: sessions)
- Table `auth_sessions` (one row per refresh-token family: trusted, user agent, client IP, created / last used,
  revoked at + reason). The migration revokes every refresh token from before it: everyone signs in again once.
- The access token carries `sid` (the family id); `AccessTokenMiddleware` checks user **and** session in one
  query on every request, so a revoked session's access tokens stop working at once ("Your session has ended.
  Please sign in again."). A token without `sid` is a 401.
- Login starts the session with its first refresh token in one statement; refresh requires a live session and
  records its use; logout and reuse detection revoke the session (tokens + row, one statement each).

## As built (part 2: devices)
- `GET /api/me/sessions` (type `sessions`): the caller's live sessions — not revoked and with a usable refresh
  token — with user agent, IP, trusted, created / last used, `expires_at` (the untrusted cap, null when trusted)
  and `current`; `sort` by `last_used_at` / `created_at`, default `-last_used_at`.
- `DELETE /api/me/sessions/:id`: locks the caller's live session and revokes it (`signed_out_remotely`) → 204;
  not a UUID / someone else's / not live → 404 "That session doesn't exist."; the current one also clears the
  cookies.
- An admin deactivating a user revokes all their sessions (`deactivated`) in the same transaction.

## Lock order (added 2026-11-12, review fix)
Every path that touches a session and its tokens locks the **session row first, then the token rows**: refresh
locks the session before the token; every revoke (logout, remote sign-out, password change / reset,
deactivation, reuse) locks the session row(s) and then revokes in a later statement, in one transaction. The
opposite orders deadlocked a refresh with a concurrent revoke (a 500). They could also leave a token, rotated in
by that refresh, live under a revoked session. That token granted nothing, since the session row decides, but
it was still drift. A derivative with sessions applies the same order.
