# 0018 — Password length is validated in bytes (`maxbytes=72`)

Status: built (2026-11-11)

## Decision
1. A password's upper bound is **72 bytes**, bcrypt's input limit, and it is checked **in the request's
   validation** with the custom tag `maxbytes=72` (`validation.MaxBytes`, registered in
   `RegisterGinValidators`) — never with `max=72`, which counts characters (runes).
2. Over the limit is a `422` on the field (its own `source.pointer`, e.g. `/data/attributes/password`), the detail
   built from the field's label: "Password must be at most 72 bytes (accented letters, non-Latin scripts and emoji
   take more than one byte each)." The wording is neutral: `maxbytes` serves any byte-limited field, an accented
   letter is still one character but takes several bytes, and the text gives no byte count per character (an emoji with a
   skin tone or a flag is 8+ bytes). Amended 2026-11-13: it used to say accented letters and emoji "count as more
   than one character" (wrong) and "use a shorter one" (fits only a password).
3. Every request that carries a password uses it: register, login, password reset and password change (0007;
   change checks both the current and the new one). The minimum stays a character count (`min=8`).
4. Any other limit that is really in bytes uses `maxbytes=N` too.

## Why
`max=72` let 40 accented letters through (40 characters, 80 bytes); `bcrypt.GenerateFromPassword` then refused
them and register answered 500 — a break of the documented contract ("8–72 characters"). bcrypt's compare also
silently ignores everything past byte 72, so allowing longer input would mean part of a password is never
checked. Validating in the request answers before any service runs and in the same shape as every other field
error; catching bcrypt's error in each service instead depends on every new flow remembering to.

## Accounts made outside the API
Login checks `maxbytes=72` too, so a password over 72 bytes is refused there before any compare. An account
whose hash was made by hand with a tool that **silently cuts** the input at byte 72 (older Python `bcrypt`, PHP
`password_hash`, some `htpasswd` builds) from a longer password is therefore **locked out**: its owner types the
full password and gets the 422. A seeded or hand-made account (an admin, a test user) uses a password of at most
72 bytes, or is hashed with the app's own `password` package (Go's bcrypt refuses longer input instead of cutting
it). Accounts made through register / reset / change can't hit this: the same rule refused them first.

## Adopting it in a derivative
Copy `validation/max_bytes.go` (+ its test), register the tag, add the `maxbytes` case to the 422 messages, and
put `maxbytes=72` on every field that is hashed with bcrypt or compared against a bcrypt hash; update `docs/API.md`.
Mapping `bcrypt.ErrPasswordTooLong` to a 422 in the service is a fine second guard, not a replacement.
