# 0020 — Auth 401s carry a code that tells the client what to do; the access cookie outlives its token

Status: built (2026-11-17). Two pieces: A = the codes, B = the cookies (B needs A).

## Decision, part A — 401 codes
Every 401 that means "don't just refresh" gets its own `apperror.Code` (status 401, title `Unauthorized`), so a
client branches on `code`, never on the `detail` text (which may change or be translated). The `detail` texts
stay as they are.

| Code | When | Client action |
|---|---|---|
| `unauthorized` (unchanged) | access token missing / bad signature / expired / wrong `typ` / no `sid`; wrong login credentials | on an authenticated call: refresh once, retry |
| `refresh_superseded` | refresh: the presented token was rotated **within `reuseGrace`** (10 s) — another tab / request won the race; the session is alive | retry the original call (the browser already holds the new cookies) — no logout, no second refresh |
| `session_expired` | refresh: no / invalid / expired / unknown refresh token, an untrusted session past its cap, an access token sent instead, a **reused** spent token (after the grace; the session is revoked) | go to login |
| `session_ended` | access-token check or refresh: the session was signed out (logout, remote sign-out, password change / reset) | go to login |
| `account_inactive` | access-token check, refresh, login (after the password matched): the account is deactivated or gone | go to login (show the detail) |

## Decision, part B — cookie lifetime
1. The `app-access-token` cookie's `Max-Age` / `Expires` = the **refresh token's** expiry (as `app-refresh-token`),
   not the access token's. The JWT inside still expires after `ACCESS_TOKEN_DURATION` and every check still
   verifies `exp`, so nothing gets longer-lived except the cookie holding an expired token. An SSR server (which
   sees HttpOnly cookies, but never `app-refresh-token`, scoped to `/api/auth`) can then tell a guest (no cookie)
   from a signed-in user whose access token just expired (cookie present, `GET /api/me` → 401 `unauthorized` →
   render "checking", the client refreshes). No extra JS-readable marker cookie.
2. A refresh that fails for good (`session_expired`, `session_ended`, `account_inactive`) **clears both cookies**,
   so a stale cookie doesn't make every later visit render "checking". `refresh_superseded` clears nothing (the
   winning request already set the new cookies).
3. Known limit: a session revoked from another device can't clear this device's cookies; its next visit renders
   "checking" once, the refresh fails and clears them.

## Why
All of them were `unauthorized`, so a client could only tell them apart by the English `detail`. Worse, a
refresh that lost a race with another tab (`refresh_superseded`) answered the same code **and** text as an
expired session ("Your session has expired. Please log in again."), so a front end with several tabs, or one
refreshing during SSR while the browser also refreshes, would log a live session out. Raised by the front-end
team integrating an SSR React app.

## Built (part A, 2026-11-16)
- `session_ended` / `account_inactive` are declared once in `middleware` (`CodeSessionEnded`,
  `CodeAccountInactive`, next to `SessionEndedMessage` and the now exported `AccountInactiveMessage`, which
  replaced the copies in refresh and login); `session_expired` / `refresh_superseded` in `refresh`. The
  middleware can't import a feature; refresh and login import it.
- `refresh_superseded` got its own detail, "Your session was just refreshed by another request. Please try
  again." — the old "Your session has expired…" would tell a user who only has to retry that they're signed out.
- `docs/API.md`: the 401 tables (*Authentication*, login, refresh), *How a session works*, the *Error code
  reference* and the *Client guide*.

## Built (part B, 2026-11-17)
- `authshared.SetTokenCookies` (login and refresh) gives the access cookie the refresh token's expiry; an untrusted
  session near its cap ends both cookies at the cap (the refresh token's expiry already is the cap).
- The refresh handler clears both cookies when the service's error is `session_expired` / `session_ended` /
  `account_inactive` (`endsSession`); not on `refresh_superseded`, not on a 500.
- **Not done, on purpose:** the access-token middleware doesn't clear cookies on its own `session_ended` /
  `account_inactive` — it would need the cookie config in `middleware`, and it wasn't promised: a client goes
  to login on those codes, and its next refresh (or login / logout) replaces or clears the cookies.
- Tests: `TestSetTokenCookies_U` (both lifetimes), `TestHandler_Refresh_U` (each terminal code clears both,
  `refresh_superseded` and a 500 touch none, a success sets equal lifetimes); both mutation-checked.
- Tests: one behaviour case per code at each place that returns it (middleware, refresh, login), asserting
  `code` and status; the concurrent-refresh test asserts `refresh_superseded` and that the session is still live.

## Adopting it in a derivative
Any repo with local sessions and a refresh endpoint: add the codes where it returns those 401s, update its API
docs, and tell its front ends. A repo without refresh / sessions takes only `account_inactive` where it applies.
