# 0005 — Cleanup worker

Status: accepted (2026-11-10), not built. Source: implementations in derivatives.

## Decision
A background worker (no endpoint) that once a day deletes rows of no further use: expired / revoked refresh
tokens, spent email tokens (0007), activity-log rows past retention (0008). It logs per-table counts, runs under
`SafeGoContext` and stops on shutdown.

## Why
Closes the *Gotcha* "`refresh_tokens` grows by one row per refresh and nothing prunes it".
