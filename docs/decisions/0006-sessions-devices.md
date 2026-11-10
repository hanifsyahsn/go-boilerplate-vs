# 0006 — Signed-in devices and remote sign-out

Status: accepted (2026-11-10), not built. Source: an implementation in a derivative.

## Decision
The caller can list the devices (sessions) they're signed in on and sign any of them out; signing out the
current one is a logout (cookies cleared). Builds on the trusted / untrusted sessions.

## Why
The natural continuation of the session model; needed by any service with its own login.
