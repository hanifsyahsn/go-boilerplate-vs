# 0005 — Cleanup worker

Status: part built (2026-11-11): the background group and shutdown order; the worker itself not yet. Source: implementations in derivatives.

## Decision
A background worker (no endpoint) that once a day deletes rows of no further use: expired / revoked refresh
tokens, spent email tokens (0007), activity-log rows past retention (0008). It logs per-table counts, runs under
`SafeGoContext` and stops on shutdown.

## Why
Closes the *Gotcha* "`refresh_tokens` grows by one row per refresh and nothing prunes it".

## As built (part 1: background group + shutdown order)
- `utility.Group` (`Go(ctx, fn)` through `SafeGoContext`, `Wait()`) and `utility.GoFunc` (what a service that
  works off the request path takes; main passes `bg.Go`).
- `main`: `bgCtx, stopBg := context.WithCancel(context.WithoutCancel(ctx))`; after `srv.Run` returned (requests
  drained) → `stopBg()` → `bg.Wait()` → the deferred DB close. A `srv.Run` error is reported only after that, so
  it never skips the flush.
