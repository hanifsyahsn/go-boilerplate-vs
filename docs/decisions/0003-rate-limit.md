# 0003 — Rate limit

Status: built (2026-11-11) — the `Limiter` interface, in-memory and Redis token buckets, the fallbacks, failed
logins per IP, the general per-IP layer.

## Decision
1. One `Limiter` interface, two implementations: **in-memory** (default) and **Redis** (used when `REDIS_URL` is set).
2. **Token bucket** everywhere (replaces the boilerplate's fixed window). "5 failed logins / 15 min per email"
   stays: a token is taken only on a failure.
3. Two layers: a general **per-IP limit on every public endpoint**, plus the strict auth limits (register, login,
   later password reset). Refined 2026-11-11: refresh and logout stay out of it (every open tab calls them, many
   users share one IP behind NAT / CGNAT, and a forged token fails the signature check before the DB); 300 / 1 min
   per IP, a flood guard only; no IP allowlist.
4. Redis down: ordinary endpoints **fail open** (warning logged); auth endpoints **fall back to the in-memory limiter**.
5. The Redis implementation is an atomic Lua token bucket with a TTL long enough for a bucket to refill, ported
   from a derivative that already runs it and reshaped to the boilerplate's conventions (slog, JSON:API 429 +
   `Retry-After`).
6. **Failed logins per client IP, across emails** (added 2026-11-11): 100 / 6 h. Credential stuffing tries each
   leaked email once, so the per-email cap never fires. Taken before the email's token; given back with a new
   `Limiter.Return` (one token, never past Burst) when the email refuses or the password is right — never a
   `Reset`, which would let one real account wipe an attacker's count. No IP allowlist: only failures count, so a
   shared IP full of people who type their password right never reaches it.

## Porting rules (agreed 2026-11-11)
The derivative's Redis limiter is **not** ported as it is:
- Redis down: auth endpoints fall back to in-memory (point 4) — never fail open on login / register / reset.
- No key read from a raw body (unbounded read, unnormalised email). Per-identifier limits live in the service,
  after the request is bound, on the normalised value.
- A missing identifier is never a 400 from the limiter; the per-IP limit still applies.
- The Lua script reads the clock with `redis.call('TIME')`, not a `now` sent by the app (clock skew between
  replicas).
- Every Redis call takes a ctx with a short timeout; a timeout counts as Redis down.
- The script takes a token atomically and returns the wait for `Retry-After`; reset is a `DEL`. No peek: a
  check now and a count later lets parallel requests all pass the check (login takes first, then refills on
  success).
- In-memory: the bucket map is capped (a new key is refused when every bucket is still refilling, so a flood
  can't wipe a victim's count) and a full map is walked at most once per token interval; IPv6 is keyed per /64.
- During a fallback, each instance counts from zero: the limit is multiplied by the replica count until Redis is
  back. Accepted for an emergency mode.

## Why
Today the boilerplate guards auth only, in memory, so every limit multiplies by the replica count. Derivatives
already run token buckets (one of them on Redis); small services shouldn't have to run Redis.

## As built (Redis + fallback)
- `ratelimit.NewRedis(client, rate, name)`: one Lua script per operation, Redis' own clock, keys `rl:<name>:<key>`
  expiring after Per + 1 s, 150 ms per call. `REDIS_URL` (optional) turns it on for every limit.
- `WithFallback(redis, memory, name)` for the auth limits (never fail open), `FailOpen(redis, name)` for the
  general per-IP limit; the "store unavailable" warning at most once per 30 s per limiter.
- The same behaviour table runs against both implementations; 50 parallel takes on a burst of 10 allow exactly 10.
