# 0021 — `ALLOWED_ORIGINS` is required outside development (`none` for no browser callers)

Status: accepted (2026-11-16), not built yet

## Decision
1. `Config.Validate()` refuses an empty `ALLOWED_ORIGINS` when `ENVIRONMENT` isn't `development` — the start fails
   with an error naming the variable, like `SMTP_HOST`. Staging counts: every environment but `development`.
2. `ALLOWED_ORIGINS=none` (alone, case-insensitive) is the explicit value for a service with **no browser front
   end** (machine-to-machine, API key): no origin is allowed, but the origin guard stays **on** — every write that
   carries an `Origin` header is a 403, a request without one (a server, curl) passes — and CORS answers no
   origin. `none` mixed with real origins is a config error.
3. In `development` (laptops and the dev server) empty stays allowed, with the existing startup warning, so a
   local try-out needs nothing.

## Why
Empty turns both CORS and the origin guard off (decision 0004 / 0009) with only a warning in the log. A derived
project that forgets the variable at deploy runs in production without the guard and nothing tells it: SameSite=Lax
still blocks a cross-site POST, but not a same-site sibling (another subdomain), which is what the guard is for.
Raised by the front-end team integrating an SSR React app. `none` exists so a service without a browser client
never has to invent a fake origin to pass the check — and it ends up stricter than empty (browser writes refused).

## Open points (settle when building)
- How the guard / CORS get "on, nothing allowed": today an empty list means off in both; `none` must reach them as
  an enabled-but-empty set (e.g. a flag on `server.Security`), not as an empty list.
- `.env.example`, CLAUDE.md *Config*, `docs/API.md` (*Cross-origin requests*), auth.md / jsonapi.md mentions of
  "empty = off".
- Tests: config cases (empty in production → error; empty in development → ok; `none` → ok; `none,https://x` →
  error); guard with `none` (write with Origin → 403, without → passes, GET passes); CORS with `none` (no header,
  preflight 403).

## Adopting it in a derivative
Every derivative: set `ALLOWED_ORIGINS` in each non-development environment (the front end's origins, or `none`)
**before** taking the code, or its next deploy refuses to start.
