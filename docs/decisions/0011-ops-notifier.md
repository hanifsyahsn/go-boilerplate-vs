# 0011 — Ops notifier

Status: accepted (2026-11-10), not built; may be deferred. Source: implementations in derivatives.

## Decision
A `Notifier` interface for operational alerts (background failures, panics), Telegram as the first
implementation, a no-op when unconfigured. Tokens from env only.
