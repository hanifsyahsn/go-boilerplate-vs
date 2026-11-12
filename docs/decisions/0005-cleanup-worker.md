# 0005 — Cleanup worker

Status: built (2026-11-11).

## Decision
A background worker (no endpoint) that once a day deletes rows of no further use: expired / revoked refresh
tokens, spent email tokens (0007), activity-log rows past retention (0008). It logs per-table counts, runs under
`SafeGoContext` and stops on shutdown.

## Why
Closes the *Gotcha* "`refresh_tokens` grows by one row per refresh and nothing prunes it".

## As built (part 1: background group + shutdown order)
- `utility.Group` (`Go(ctx, fn)`, recovered like `SafeGoContext` and counted done only after a panic was logged and
  reported; `Wait()`) and `utility.GoFunc` (what a service that
  works off the request path takes; main passes `bg.Go`).
- `main`: `bgCtx, stopBg := context.WithCancel(context.WithoutCancel(ctx))`; after `srv.Run` returned (requests
  drained) → `stopBackground`: `stopBg()` → `bg.Wait()` → the alert sender's own ctx and group (so an alert a job
  raises while finishing still goes out) → the deferred DB close. A `srv.Run` error is reported only after that, so
  it never skips the flush.

## As built (part 2: the worker)
- `cleanup.Worker` in main's background group: a minute after start, then every 24 h, on every replica. In
  batches of 10,000 until a short one: refresh tokens past expiry or session cap (never a revoked-but-unexpired
  one), sessions with no token left, expired one-time tokens, activity rows past `ACTIVITY_LOG_RETENTION` (0 =
  keep). One table's error doesn't stop the others; a done ctx stops between batches.
