# 0008 — API activity log

Status: accepted (2026-11-10), not built. Source: the most complete of the derivatives' implementations.

## Decision
One row per API request (who, which endpoint, when, outcome, request id). A middleware hands it to a recorder
that queues and writes in batches off the request path and flushes on shutdown; rows past
`ACTIVITY_LOG_RETENTION` are deleted by 0005. Derivatives with their own version move to this one.

## Why
Several derivatives built it separately; one shared version instead. The shutdown flush must be tested.
