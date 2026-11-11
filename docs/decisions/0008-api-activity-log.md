# 0008 — API activity log

Status: built (2026-11-11). Source: the most complete of the derivatives' implementations.

## Decision
One row per API request (who, which endpoint, when, outcome, request id). A middleware hands it to a recorder
that queues and writes in batches off the request path and flushes on shutdown; rows past
`ACTIVITY_LOG_RETENTION` are deleted by 0005. Derivatives with their own version move to this one.

## Why
Several derivatives built it separately; one shared version instead. The shutdown flush must be tested.

## As built
- Table `api_activity_logs` (when, request id, user / session — NULL when anonymous, no FK —, method, matched
  route, path without query string, status, latency, response size, client IP, user agent). No body, no query.
- `middleware.ActivityLog` right after `AccessLog` (404 / 405 / panics recorded, `/healthz` skipped);
  `activitylog.Recorder` never blocks (4096 queue, drops counted and logged), writes batches of 200 or every 2 s
  in one `INSERT … unnest` statement, drains and writes the queue at shutdown (`TestRecorder_ShutdownFlush_I`).
- `ACTIVITY_LOG_RETENTION` (default 2 years, 0 = forever) is applied by the cleanup worker (0005).
