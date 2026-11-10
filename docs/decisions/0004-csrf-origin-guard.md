# 0004 — CSRF `OriginGuard`

Status: accepted (2026-11-10), not built. Source: an implementation in a derivative.

## Decision
A state-changing request (not GET / HEAD / OPTIONS) that carries an `Origin` header must come from an allowed
origin, else 403. No `Origin` (curl, a server) passes. Allowlist from env, shared with 0009; empty = off.

## Why
The refresh token rides a cookie; SameSite=Lax alone doesn't close every CSRF path.
