# 0006 — Signed-in devices and remote sign-out

Status: part built (2026-11-11): sessions + immediate sign-out; the list / revoke endpoints not yet. Source: an implementation in a derivative.

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
