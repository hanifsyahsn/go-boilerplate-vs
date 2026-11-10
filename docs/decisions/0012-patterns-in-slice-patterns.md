# 0012 — Derivative patterns into `slice-patterns.md`

Status: accepted (2026-11-10), not built.

## Decision
Add as reference patterns (not core code): `adminscope` (department-scoped admin, other departments read as 404),
`failure` (a user-facing message for a background failure, technical cause kept), an access-request approval
flow (create / approve / reject / cancel / queue), and `GET /app-config` (server flags the frontend follows).
