# 0007 — Forgot / change password, email verification, `Mailer`

Status: built (2026-11-11). Source: an implementation in a derivative.

## Decision
- Forgot password: email a one-time link (1 h), then set a new password (revokes the user's sessions).
- Change password: the current password is required.
- Email verification for registrants + resend.
- A `Mailer` interface in platform (SMTP implementation + a fake for tests).
- These endpoints sit under the auth rate limits (0003); spent tokens are pruned by 0005.

## Why
A prerequisite for any service with its own local users.

## As built (part 1: mailer + one-time tokens)
- `internal/platform/mailer`: `Mailer` with `NewSMTP` (465 implicit TLS, else STARTTLS required, 15 s per email,
  CR / LF in a header refused, the password never in an error), `NewLog` for development and `Fake` for tests.
- `internal/shared/onetimetoken`: 32 random bytes as 43 base64url characters; only the hex SHA-256 is stored.
- Migration: `users.email_verified_at`, table `auth_tokens` (purpose `VERIFY_EMAIL` / `RESET_PASSWORD`, unique
  hash, used_at). Issuing spends the user's unused tokens of that purpose; consuming is one atomic `UPDATE`.
- Config: `SMTP_HOST`, `SMTP_PORT` (587), `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`, `APP_BASE_URL` (required;
  SMTP host and sender optional only in development).

## As built (part 2: forgot password)
- `POST /api/auth/password-reset-requests`: always 202 with one message (unknown email, inactive account, the
  3 / h per-address limit and a failed send look the same); an active account gets a 1 h link by email, earlier
  links spent.
- **Same time for every email (added 2026-11-12):** the request answers before any work. The limit, the lookup,
  the token and the email all run in the background group afterwards. A lookup or token write before answering
  made a known email measurably slower than an unknown one. A DB failure is therefore logged, never a 500: a
  500 would itself tell the emails apart. A derivative with forgot-password applies the same shape.
- `POST /api/auth/password-resets`: consumes the token atomically (else 422 `invalid_token` on the token), sets
  the password, marks the email verified and ends every session in one transaction, then emails a notice → 204.
- Both in the auth per-IP group (`limited`).

## As built (part 3: change password)
- `POST /api/auth/password-changes` (authed): the current password is required (422 `wrong_password`), wrong
  ones capped at 5 / 15 min per user (429, taken before the compare); the new password and the end of every
  **other** session in one transaction → 204, the caller stays signed in; a notice email follows.

## As built (part 4: email verification)
- Register issues a 24 h `VERIFY_EMAIL` token in its transaction and emails the link after commit; login isn't
  blocked for an unverified email; `GET /api/me` carries `email_verified`.
- `POST /api/auth/email-verifications` (public, auth per-IP group): consume → verified → 204; else the same 422
  `invalid_token`.
- `POST /api/me/email-verification-requests` (authed, body optional): 409 `email_already_verified`, 3 / h per
  user (429), else a new link → 202.
