# 0003 — Rate limit

Status: accepted (2026-11-10), not built

## Decision
1. One `Limiter` interface, two implementations: **in-memory** (default) and **Redis** (used when `REDIS_URL` is set).
2. **Token bucket** everywhere (replaces the boilerplate's fixed window). "5 failed logins / 15 min per email"
   stays: a token is taken only on a failure.
3. Two layers: a general **per-IP limit on every public endpoint**, plus the strict auth limits (register, login,
   later password reset).
4. Redis down: ordinary endpoints **fail open** (warning logged); auth endpoints **fall back to the in-memory limiter**.
5. The Redis implementation is an atomic Lua token bucket with a TTL long enough for a bucket to refill, ported
   from a derivative that already runs it and reshaped to the boilerplate's conventions (slog, JSON:API 429 +
   `Retry-After`).

## Why
Today the boilerplate guards auth only, in memory, so every limit multiplies by the replica count. Derivatives
already run token buckets (one of them on Redis); small services shouldn't have to run Redis.
