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
  raises while finishing still goes out; at most 10 s, inside deploy.sh's 30 s) → the deferred DB close. A `srv.Run` error is reported only after that, so
  it never skips the flush.

- **Amended 2026-11-27: one deadline over jobs and alerts.** `bg.Wait()` had no bound, so a job that ignored its
  cancel held the process until deploy.sh's force stop (30 s after SIGTERM), and the last alerts died with it.
  `stopBackground` now runs under `backgroundStopTimeout` = 20 s from its call (server drain ≤ 5 s + 20 s, 5 s left
  to close the pools and exit): the jobs are waited for up to the deadline, then given up with warn `background:
  jobs not finished before shutdown`; the alerts get what's left, at most 10 s, then warn `notifier: alerts not
  sent before shutdown`. Adopting it: give every background job's stop path the deadline, sized to that repo's own
  force-stop budget.

## As built (part 2: the worker)
- `cleanup.Worker` in main's background group: a minute after start, then every 24 h, on every replica. In
  batches of 10,000 until a short one: refresh tokens past expiry or session cap (never a revoked-but-unexpired
  one), sessions with no token left, expired one-time tokens, activity rows past `ACTIVITY_LOG_RETENTION` (0 =
  keep). One table's error doesn't stop the others; a done ctx stops between batches.
