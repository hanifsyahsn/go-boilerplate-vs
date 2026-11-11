---
paths:
  - "internal/features/auth/**"
  - "internal/features/me/**"
  - "internal/features/admin/**"
  - "internal/platform/middleware/access_token*"
  - "internal/platform/middleware/require_role*"
  - "internal/platform/middleware/rate_limit*"
  - "internal/platform/ratelimit/**"
  - "internal/platform/server/**"
  - "internal/platform/config/**"
  - ".semgrep/**"
  - "internal/platform/jwttoken/**"
  - "internal/shared/password/**"
  - "internal/shared/enum/**"
  - "internal/platform/db/migration/000002_*"
  - "internal/platform/db/migration/000004_*"
  - "internal/platform/db/migration/000005_*"
---
# Auth: register, login, refresh, logout

Moved verbatim from CLAUDE.md on 2026-11-04. Related rules: jsonapi.md (request documents, the `sessions` /
`users` types), logging.md (`user_id` on the request context). What applies everywhere is in the root CLAUDE.md.

### Auth

- **Password length is in bytes** (decision 0018): register and login validate `maxbytes=72` (`validation.MaxBytes`),
  never `max=72` — `max` counts characters, bcrypt refuses over 72 **bytes**, so 40 accented letters passed `max`
  and register answered 500. Register's minimum stays `min=8` characters.
- Register takes a `users` document; role is always `user` (a `role` attribute is a 403); email unique
  ignoring case (`users_email_lower_key`, pq `23505` → 409). 201 with the account, no `Location` (a plain user
  has no URL of its own).
- Login takes a `sessions` document (`email`, `password`). One 401 message for unknown email and wrong
  password; "no longer active" only after the password matched. Sets `app-access-token` (path `/`) and
  `app-refresh-token` (path `/api/auth`) HttpOnly, SameSite=Lax cookies (`COOKIE_SECURE`, `COOKIE_DOMAIN`), and
  answers with the account as `data` and the tokens under `meta` for non-browser clients.
- Tokens carry `typ` (`access` / `refresh`); each verify rejects the other type.
- **Refresh tokens rotate** (migration 000004, table `refresh_tokens`, one row per refresh token keyed on its
  `jti`). Login starts a *family* (one per login / device). `POST /api/auth/refresh` locks the presented token's
  row, issues a **new pair** (both cookies replaced, both tokens in `meta`), inserts the successor in the same
  family and marks the old one `revoked_at` + `replaced_by`. The new refresh token lives
  `REFRESH_TOKEN_DURATION` (optional, default 7 days) from now, so it's an idle timeout: a user who keeps
  coming back is never logged out; one who's away longer logs in again.
- **Trusted vs untrusted device** (migration 000005): login's optional `trust_device` attribute. Trusted →
  `refresh_tokens.session_expires_at` NULL, the session slides with no cap. Untrusted (the default) →
  `session_expires_at = login + UNTRUSTED_SESSION_DURATION` (optional, default 7 days), copied to every
  successor; refresh issues a token no longer than what's left and is a 401 past it.
- **Reuse detection**: a spent token presented again revokes its whole family (→ 401), since it was probably
  stolen. Exception: within `reuseGrace` (10s, `refresh/service.go`) of its rotation it's treated as two tabs
  refreshing at once — 401, family kept. Clients should still single-flight their refresh calls.
- Refresh re-reads the user, so the new access token carries the **current** role and a deactivated user gets
  none. An unknown jti (no row) is a 401.
- Refresh / logout take the token from the cookie, else an optional `sessions` document
  (`{"data":{"type":"sessions","attributes":{"refresh_token":"..."}}}`). Refresh answers with a meta-only
  document.
- Logout revokes the presented token's family and clears the cookies; a missing / invalid token is still a
  204. The access token is a stateless JWT and stays valid until it
  expires — keep `ACCESS_TOKEN_DURATION` short.

### Rate limits and the client IP

Added 2026-11-09 as the boilerplate's base security.

- **Client IP = `c.ClientIP()`, never a header read by hand.** `server.NewRouter` calls
  `r.SetTrustedProxies(Security.TrustedProxies)` (`TRUSTED_PROXIES`); empty passes `nil` = trust no proxy, so the
  IP is the connection's address and a forged `X-Forwarded-For` changes nothing. gin's default (no call) trusts
  **every** peer — that's what let a client pick its own IP. Behind nginx / a load balancer, list its IP or CIDR,
  else every client shares the proxy's IP. Each entry is validated in `Config.Validate()` (IP or CIDR) and again
  by gin; `NewRouter` / `NewServer` return the error and `main` stops.
- **`internal/platform/ratelimit.Limiter`** (decision 0003): a **token bucket** per key behind an interface —
  `Take` (take a token if there is one, **atomically**), `Reset` (refill) and `Return` (give **one** token
  back, never past Burst); each takes a ctx and can return an error (a shared store that's down); `Take` answers
  a `Decision{Allowed, RetryAfter}`. **No peek-then-take**: a check now and a count later is a race (N parallel
  requests all pass the check) — take first, give back with `Reset` / `Return`. A
  `Rate{Burst, Per}` is Burst tokens refilled evenly over Per: `{5, 15m}` = five at once, then one every 3 min.
  `ratelimit.NewMemory(rate)` is the in-process one (never errors; one instance only, see *Gotchas* in
  CLAUDE.md). It keeps only buckets still refilling (a full one is dropped; swept once per `Per`, and while the
  map is full once per token interval — not on every request, which a flood could use to burn CPU) and at most `DefaultMaxKeys` (100,000) of them: when all are still refilling, a **new** key is
  refused (warn `rate limit: bucket map full, new key refused`) rather than an old one forgotten — forgetting
  would let a flood of fresh keys wipe a victim's count. Keys of a client IP go through `ratelimit.IPKey`: IPv4
  as is, **IPv6 per /64** (one subscriber gets a whole /64).
- **Per IP** (`middleware.RateLimitByIP(limiter, "auth")`, `Security.AuthIPLimit`, **required** — `NewRouter`
  refuses nil): register and login share one bucket, 300 / 15 min per IP (`authIPRate` in `main.go`; generous
  because an office NAT shares one IP). Past it → 429 `too_many_requests` with `Retry-After` (whole seconds,
  rounded up — `middleware.RetryAfterSeconds`), detail `middleware.TooManyRequestsMessage`, the handler never
  runs. A limiter error is a 500 (the limiter owns surviving its store — decision 0003). **Refresh and logout are not IP-limited**: they run
  off the refresh cookie on every open tab.
- **Per email, failed logins** (`login` service, its own limiter, 5 / 15 min — `loginFailureRate`): key
  `login:` + trimmed, lowercased email (as `GetUserForLogin` matches it). Every attempt `Take`s a token
  **before the DB read** (so parallel guesses can't outrun the count — `TestService_Login_ParallelGuesses_U`);
  an unknown email and a wrong password keep it (so a 429 doesn't reveal whether the email exists); the right
  password `Reset`s the bucket — a deactivated account included, it isn't a guess — so in effect only failures
  count. Once empty, every attempt for that email is a 429, the right password too, until a token comes back
  (3 min). A limiter error is a 500 and a refused key (empty bucket, or a full bucket map) a 429 — fail closed,
  never an uncounted guess; a failed `Reset` after the right password is only logged. The login 429 carries no `Retry-After` yet. Known trade-off: anyone can keep an email locked out
  by spending a token every 3 min.
- **Per IP, failed logins across emails** (`login` service, `Limiters.IP`, 100 / 6 h — `loginIPFailureRate`): key
  `loginip:` + `ratelimit.IPKey(c.ClientIP())` (the handler passes `login.Client{IP, UserAgent}`). Credential
  stuffing tries each leaked email once, so the per-email cap never fires; this one does. The IP's token is
  taken **first**, then the email's, both before the DB read: a caller the IP cap already stopped can't drain a
  victim's email budget, and when the **email** refuses (or errors) the IP gets its token back with `Return` — no
  password was tried, so hammering a locked email from a shared IP doesn't lock the IP. The right password
  `Return`s the IP's token — **never `Reset`**: one real account would otherwise wipe an attacker's count. Same
  429 message as the per-email cap; a limiter error is a 500; a failed give-back is logged (warn
  `login: give back failure count failed`, `limiter` = `email` / `ip`). No IP allowlist (an office NAT only
  reaches it with 100 wrong passwords). Tests: `TestService_Login_IPFailureLimit_U`,
  `TestService_Login_LimiterOutcomes_U`, `TestService_Login_ParallelGuesses_U` (20 emails from one IP).
- **A new public endpoint** (no access token) goes into the `limited` group in `router.go` unless, like
  refresh, it runs automatically per tab. **One that sends email** (verify link, password reset) also needs a
  per-recipient limit in its service (e.g. 3 / h), or it can be used to spam any address through our SMTP.
- Tests: `ratelimit/limiter_unit_test.go` (`TestMemory_U`, `TestMemory_Take_Parallel_U`, `TestIPKey_U`), `middleware/rate_limit_unit_test.go`,
  `server/router_unit_test.go` (`TestNewRouter_Security_U` — which
  routes are limited, forged vs trusted `X-Forwarded-For`; `TestNewRouter_SecurityErrors_U`),
  `login/service_unit_test.go` (`TestService_Login_FailureLimit_U`,
  `TestService_Login_LimiterOutcomes_U`, `TestService_Login_ParallelGuesses_U`), config's `TRUSTED_PROXIES` cases.
  Integration routers pass a high limit (`testLimit`) so multi-login flows from httptest's one address pass.
- `make sec SEC_PASSWORD=imsureitislocal` (local only — see *Gotchas* in CLAUDE.md; gosec without generated code, semgrep with `p/golang` + `.semgrep/`, govulncheck, gitleaks over git
  history) is the security scan; it runs all four and fails at the end if any failed, and must stay green (see
  *Gotchas* in CLAUDE.md for `#nosec` / `nosemgrep`). `.semgrep/gin.yml` bans `gin.Default()` (trusts every
  proxy) and `context.WithoutCancel` on a `*gin.Context`. A convention worth enforcing gets a rule there.
- **Login timing**: an unknown email still spends one bcrypt comparison (`password.CheckOrWaste("", …)` against a
  dummy hash), so the response time doesn't reveal which emails have an account; `password.Check` never matches
  an empty hash. `TestService_Login_TimingEqualized_U` pins it.
