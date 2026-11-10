# 0007 — Forgot / change password, email verification, `Mailer`

Status: accepted (2026-11-10), not built. Source: an implementation in a derivative.

## Decision
- Forgot password: email a one-time link (1 h), then set a new password (revokes the user's sessions).
- Change password: the current password is required.
- Email verification for registrants + resend.
- A `Mailer` interface in platform (SMTP implementation + a fake for tests).
- These endpoints sit under the auth rate limits (0003); spent tokens are pruned by 0005.

## Why
A prerequisite for any service with its own local users.
