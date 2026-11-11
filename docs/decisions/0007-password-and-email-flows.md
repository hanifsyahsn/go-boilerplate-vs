# 0007 — Forgot / change password, email verification, `Mailer`

Status: part built (2026-11-11): the mailer, one-time tokens and forgot password; change password and email verification not yet. Source: an implementation in a derivative.

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
- `POST /api/auth/password-resets`: consumes the token atomically (else 422 `invalid_token` on the token), sets
  the password, marks the email verified and ends every session in one transaction, then emails a notice → 204.
- Both in the auth per-IP group (`limited`).
