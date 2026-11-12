---
paths:
  - "internal/features/auth/**"
  - "internal/features/me/**"
  - "internal/features/admin/**"
  - "internal/features/session/**"
  - "internal/platform/middleware/access_token*"
  - "internal/platform/middleware/require_role*"
  - "internal/platform/middleware/rate_limit*"
  - "internal/platform/ratelimit/**"
  - "internal/platform/server/**"
  - "internal/platform/config/**"
  - ".semgrep/**"
  - "internal/platform/jwttoken/**"
  - "internal/shared/password/**"
  - "internal/shared/onetimetoken/**"
  - "internal/platform/mailer/**"
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
- **CSRF**: the cookies are SameSite=Lax **and** `/api` runs `middleware.OriginGuard` (decision 0004): a write
  whose `Origin` isn't in `ALLOWED_ORIGINS` is a 403 before anything else runs. Lax alone leaves a same-site
  sibling (another subdomain) and old browsers; a request without `Origin` passes (no page sent it). Empty
  `ALLOWED_ORIGINS` turns the guard off (a startup warning says so) — set it in every deployed environment.
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
- **Reuse detection**: a spent token presented again revokes its whole session (`token_reused`, → 401), since
  it was probably stolen. Exception: within `reuseGrace` (10s, `refresh/service.go`) of its rotation it's treated as two tabs
  refreshing at once — 401, family kept. Clients should still single-flight their refresh calls.
- Refresh re-reads the user, so the new access token carries the **current** role and a deactivated user gets
  none. An unknown jti (no row) is a 401.
- Refresh / logout take the token from the cookie, else an optional `sessions` document
  (`{"data":{"type":"sessions","attributes":{"refresh_token":"..."}}}`). Refresh answers with a meta-only
  document.
- **Sessions** (decision 0006, migration 000006, table `auth_sessions`): one row per refresh-token family =
  one login on one device (`trusted`, `user_agent` cut to 512 runes, `client_ip`, `created_at`,
  `last_used_at`, `revoked_at` + `revoke_reason`). Login starts it together with the first refresh token in
  **one statement** (`authshared` query `StartSession`); refresh requires it live (`LockRefreshToken` joins it:
  a signed-out session is a 401 "Your session has ended. Please sign in again." — `middleware.SessionEndedMessage`
  — or "no longer active" when the account is inactive / gone, checked first: the same details and order as the
  access-token check; not treated as reuse) and `TouchSession`s it (last use, User-Agent, IP).
  The access token carries `sid` = the family id; `AccessTokenMiddleware` reads user and session in **one**
  query (`GetAccessStatus`) on every request — no / bad `sid` → 401 "not authorized" (tokens from before
  sessions: everyone signs in again once), inactive → "no longer active", session revoked → **"Your session
  has ended. Please sign in again."** — and sets gin key `session_id` (`constant.SessionIDKey`, a
  `uuid.UUID`). Ending sessions goes through two shared queries only, each revoking the tokens **and** the
  session row in one statement: `RevokeSession(family_id, reason)` and `RevokeUserSessions(user_id, reason,
  keep_family_id NULL = all)`; reasons are the `authshared.RevokeReason*` constants (the column's CHECK).
- **Lock order: the session row, then its token rows — everywhere** (review fix 2026-11-12). Refresh runs
  `LockSessionOfToken` before `LockRefreshToken`. A revoke never runs `RevokeSession` / `RevokeUserSessions`
  alone: it goes through `authshared.EndSession` / `EndUserSessions` (lock the session row(s) — many in
  `family_id` order — then revoke, in a transaction), or a slice's own session lock first (`session/revoke`'s
  `LockLiveSession`; refresh's reuse path already holds it). The opposite orders deadlocked (Postgres aborts one
  → a 500). The revoke is a later statement than the lock, so it also sees a token a concurrent refresh
  rotated in. Logout has a transaction for this. A new path that touches a session and its tokens follows the
  same order. Tests: `TestService_Refresh_RevokeInFlight_I`, `TestEndSessions_RefreshInFlight_I`,
  `TestService_Logout_RefreshInFlight_I` (`testhelper.BeginRotation` / `LockSessionRow` hold the locks).
- Logout revokes the presented token's session (`logout`) and clears the cookies; a missing / invalid /
  unknown token is still a 204. The session's access tokens stop working at once.
- **Signed-in devices** (slices `session/list`, `session/revoke`, resource type `sessions`, shared in
  `session/shared`): `GET /api/me/sessions` lists the caller's **live** sessions — not revoked and with a refresh
  token still usable (not revoked, not expired, inside `session_expires_at`) — sorted `-last_used_at` by default
  (`last_used_at` / `created_at`), `expires_at` = the untrusted cap or null, `current` = the request's `sid`.
  `DELETE /api/me/sessions/:id` locks the caller's live session (`LockLiveSession`, `FOR UPDATE`) and
  `RevokeSession(signed_out_remotely)` → 204; a non-UUID id, someone else's or a non-live session → 404 "That
  session doesn't exist." (not-yours reads as not-found); the current `sid` also clears the cookies (a logout).
- **Deactivating a user** (`PATCH /api/admin/users/:id`, `is_active: false`) revokes all their sessions
  (`deactivated`) in the same transaction, so reactivating never brings an old device back.

- **One-time email tokens** (decision 0007, migration 000007): table `auth_tokens` (`purpose` `VERIFY_EMAIL` 24 h
  / `RESET_PASSWORD` 1 h, only the SHA-256 of the link's token is stored — `onetimetoken`), `users.email_verified_at`.
  `authshared.IssueAuthToken` (in the caller's tx) spends the user's unused tokens of that purpose, then inserts
  the new one — only the latest link works; `ConsumeAuthToken` is the atomic single use (`UPDATE … WHERE unused
  AND unexpired RETURNING user_id`: two parallel requests with one token → one row). Emails go out through
  `authshared.Emailer.Send` — off the request path in main's background group (`utility.GoFunc`), a failure logged
  `send email failed` with `purpose` and `recipient_id`, **never the address or the link**. Texts and links
  (`APP_BASE_URL + "/reset-password?token="`, `"/verify-email?token="`) are `authshared.ResetEmail` /
  `PasswordChangedEmail` / `VerifyEmail`.
- **Forgot password** (slices `auth/passwordresetrequest`, `auth/passwordreset`, both in the `limited` group):
  `POST /api/auth/password-reset-requests` (`password-reset-requests`, attribute `email`) is **always 202** with
  one meta message, **answered before any work** (Q16, 2026-11-12): `Request` only hands the work to main's
  background group (`emailer.Go`, `requestid.Detach`) and returns nothing, so a known and an unknown email take
  the same time (a lookup or token write before answering told them apart). In the background: the
  per-recipient limit (3 / h, key `reset:` + the trimmed email folded by Postgres (`FoldEmail`), taken **before** the lookup; refused or
  failing → nothing sent), the lookup, a `RESET_PASSWORD` token (1 h) for an active account, the email. Every
  failure (`password reset: recipient limit failed` / `lookup failed` / `issue token failed`) is only logged,
  never with the address; there is no 500. Tests: `TestService_Request_AnswersBeforeWork_U`,
  `TestService_Request_BackgroundGroup_I`.
  `POST /api/auth/password-resets` (`password-resets`, `token` + `password` `min=8,maxbytes=72`): in one tx
  `ConsumeAuthToken` (none → 422 `invalid_token` on `/data/attributes/token`, `authshared.CodeInvalidToken`), new
  hash + `email_verified_at` (`SetPasswordAfterReset`), `RevokeUserSessions(password_reset)`; bcrypt runs before
  the tx; after commit the "password changed" notice → 204.
- **Change password** (slice `auth/passwordchange`, authed, `POST /api/auth/password-changes`, `password-changes`:
  `current_password` `maxbytes=72`, `password` `min=8,maxbytes=72`): the per-user failure limiter (5 / 15 min, key
  `pwchange:` + user id) is **taken first** (refused → 429 "Too many wrong attempts…", error → 500), a wrong
  current password → 422 `wrong_password` on `/data/attributes/current_password`, the right one `Reset`s the count
  (a failed reset only logged); then in one tx the new hash + `RevokeUserSessions(password_changed, keep = the
  caller's sid)` → 204, the caller stays signed in; then the notice (log label `PASSWORD_CHANGED`).
- **Email verification** (boilerplate shape): register issues a `VERIFY_EMAIL` token (24 h) in its insert tx
  and emails the link after commit; login is **not** blocked for an unverified email, `GET /api/me` carries
  `email_verified`. `POST /api/auth/email-verifications` (`auth/emailverification`, `limited` group,
  `email-verifications`, `token`): consume → `email_verified_at = COALESCE(…, now())` → 204, else the same 422
  `invalid_token`. `POST /api/me/email-verification-requests` (`auth/emailverificationrequest`, authed, body
  optional): verified → 409 `email_already_verified`, then the per-user limit (3 / h, key `verify:` + id → 429),
  then issue + send → 202 meta "We've sent a new verification link.". A derivative with its own registration
  flow keeps its domain flow and reuses `onetimetoken`, `auth_tokens`, `ConsumeAuthToken`, `mailer` and the 422.
- **`mailer`**: `NewSMTP` refuses to send without TLS (465 implicit, else STARTTLS required, certificate verified),
  15 s per email, a header with CR / LF refused; `NewLog` (development, no `SMTP_HOST`) logs to + subject at info,
  the body only at debug; `mailer.Fake` records messages for tests.

- **Machine callers** (decision 0010): `middleware.APIKey(conf.APIKeys)` on a group of machine-only routes — the
  boilerplate mounts none. Header `X-API-Key`, exactly one and non-empty (two headers → 401); its SHA-256 is compared with every configured key's in constant
  time, over **all** keys (no early exit); none matches → 401 "A valid API key is required." on `source.header`
  `X-API-Key`; a match sets gin key `api_client` (the name), which `AccessLog` adds to its record and the activity log stores (`api_client`) — never the key.
  Never mount it next to the cookie auth on one route: a machine route takes no user session.

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
  `ratelimit.NewRedis(client, rate, name)` (with `REDIS_URL`) is the shared one: the same bucket as one Lua script
  per operation (`take.lua`, `return.lua`, a `DEL` for Reset), timed by Redis' `TIME`, keys `rl:<name>:<key>`
  expiring after Per + 1 s, every call bounded by `RedisTimeout` (150 ms). Main wraps it: auth limits in
  `ratelimit.WithFallback(redis, memory, name)` (Redis error → the memory one decides; Reset / Return hit both),
  the general per-IP limit in `ratelimit.FailOpen(redis, name)` (error → allowed); both warn `rate limit: store
  unavailable` (`limiter`, `mode`) at most once per 30 s. One behaviour table runs against both implementations
  (`runLimiterContract`: `TestMemory_Contract_U`, `TestRedis_Contract_I`), plus `TestRedis_Take_Parallel_I`.
  `ratelimit.NewMemory(rate)` is the in-process one (never errors; one instance only, see *Gotchas* in
  CLAUDE.md; like `NewRedis` it panics on a rate whose token interval `Per / Burst` is zero). It keeps only buckets still refilling (a full one is dropped; swept once per `Per`, and while the
  map is full once per token interval — not on every request, which a flood could use to burn CPU) and at most `DefaultMaxKeys` (100,000) of them: when all are still refilling, a **new** key is
  refused (warn `rate limit: bucket map full, new key refused`) rather than an old one forgotten — forgetting
  would let a flood of fresh keys wipe a victim's count. Keys of a client IP go through `ratelimit.IPKey`: IPv4
  as is, **IPv6 per /64** (one subscriber gets a whole /64).
- **Per IP** (`middleware.RateLimitByIP(limiter, "auth")`, `Security.AuthIPLimit`, **required** — `NewRouter`
  refuses nil, a typed nil such as a nil `*ratelimit.Memory` included): register and login share one bucket,
  300 / 15 min per IP (`authIPRate` in `main.go`; generous
  because an office NAT shares one IP). Past it → 429 `too_many_requests` with `Retry-After` (whole seconds,
  rounded up — `apperror.RetryAfterSeconds`, set from the error's `WithRetryAfter`), detail
  `middleware.TooManyRequestsMessage`, the handler never runs. A limiter error is a 500 (the limiter owns surviving its store — decision 0003). **Refresh and logout are not IP-limited**: they run
  off the refresh cookie on every open tab.
- **General per IP** (`middleware.RateLimitByIP(limiter, "public")`, `Security.PublicIPLimit`, **required** —
  `NewRouter` refuses nil): every public route **except refresh and logout** (router group `guarded`; `limited`
  is its child, so register / login spend from both), 300 / 1 min per IP (`publicIPRate`). A flood guard only:
  many users share one IP (office NAT, mobile carrier CGNAT), so it's generous — the per-email / per-IP failure
  limits are what stop guessing. Refresh / logout stay out (every open tab calls them; a forged token fails the
  signature check before the DB). Authenticated routes aren't IP-limited. Same 429 + `Retry-After` as the auth one.
- **Per email, failed logins** (`login` service, its own limiter, 5 / 15 min — `loginFailureRate`): key
  `login:` + the trimmed email **folded by Postgres** (`authshared` query `FoldEmail`, `SELECT lower($1)`), exactly
  as `GetUserForLogin` matches it. Never Go's `strings.ToLower` for a key a SQL `lower()` lookup must agree with:
  their Unicode tables differ (glibc lowers U+A7DC to `ƛ`, Go doesn't), so one account got two counts
  (`TestService_Login_FoldedLikeTheLookup_I`). Login takes the IP's token first (it needs no email, so a login the IP cap refuses costs no query), then folds
  (an error is a 500 and gives the IP its token back), then the email's token; the fold and the lookup get the
  same trimmed email. A failed account lookup (a DB error, not "no such email") gives both tokens back with `Return`: no password
  was tried. Every give-back (`Return` / `Reset`, in login and password change) runs on
  `context.WithoutCancel(ctx)`: a client that hung up cancels the request ctx, Redis refuses a call on it, and the
  count would stay taken although no password was tried or the right one was (`TestService_Login_GiveBackOutlivesRequest_U`,
  `TestService_Change_ResetOutlivesRequest_U`). Every
  attempt `Take`s a token
  **before the DB read** (so parallel guesses can't outrun the count — `TestService_Login_ParallelGuesses_U`);
  an unknown email and a wrong password keep it (so a 429 doesn't reveal whether the email exists); the right
  password `Reset`s the bucket — a deactivated account included, it isn't a guess — so in effect only failures
  count. Once empty, every attempt for that email is a 429, the right password too, until a token comes back
  (3 min). A limiter error is a 500 and a refused key (empty bucket, or a full bucket map) a 429 — fail closed,
  never an uncounted guess; a failed `Reset` after the right password is only logged. The 429 carries the
limiter's wait (`WithRetryAfter(d.RetryAfter)` → `Retry-After`). Known trade-off: anyone can keep an email locked out
  by spending a token every 3 min.
- **Per IP, failed logins across emails** (`login` service, `Limiters.IP`, 100 / 6 h — `loginIPFailureRate`): key
  `loginip:` + `ratelimit.IPKey(c.ClientIP())` (the handler passes `login.Client{IP, UserAgent}`). Credential
  stuffing tries each leaked email once, so the per-email cap never fires; this one does. The IP's token is
  taken **first** (before the fold too), then the email's, both before the account lookup: a caller the IP cap already stopped can't drain a
  victim's email budget, and when the **email** refuses (or errors) the IP gets its token back with `Return` — no
  password was tried, so hammering a locked email from a shared IP doesn't lock the IP. The right password
  `Return`s the IP's token — **never `Reset`**: one real account would otherwise wipe an attacker's count. Same
  429 message as the per-email cap; a limiter error is a 500; a failed give-back is logged (warn
  `login: give back failure count failed`, `limiter` = `email` / `ip`). No IP allowlist (an office NAT only
  reaches it with 100 wrong passwords). Tests: `TestService_Login_IPFailureLimit_U`,
  `TestService_Login_LimiterOutcomes_U`, `TestService_Login_ParallelGuesses_U` (20 emails from one IP).
- **Every 429 says how long to wait** (decision 0003, Q18 2026-11-12): a rate-limit refusal is
  `apperror.New(apperror.CodeTooManyRequests, msg, err).WithRetryAfter(d.RetryAfter)` (the limiter's `Decision`),
  in a service as in `RateLimitByIP`. `jsonapi.WriteErrorsContext` turns it into a `Retry-After` header (whole
  seconds, rounded up, at least 1; the longest wait of several errors), and CORS exposes it. A service limit
  that answers 429 without it is a bug. Tests: `TestWriteErrors_RetryAfter_U`, `TestWriteError_U`, and each
  service's 429 case asserts `RetryAfter`.
- **A new public endpoint** (no access token) goes into the `guarded` group in `router.go` (the general per-IP
  limit), or its `limited` child when it touches credentials or sends email; only one that runs automatically per
  tab, like refresh, goes straight into `public`. **One that sends email** (verify link, password reset) also needs a
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
