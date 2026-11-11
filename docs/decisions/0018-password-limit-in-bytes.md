# 0018 — Password length is validated in bytes (`maxbytes=72`)

Status: built (2026-11-11)

## Decision
1. A password's upper bound is **72 bytes**, bcrypt's input limit, and it is checked **in the request's
   validation** with the custom tag `maxbytes=72` (`validation.MaxBytes`, registered in
   `RegisterGinValidators`) — never with `max=72`, which counts characters (runes).
2. Over the limit is a `422` on the field (`source.pointer` `/data/attributes/password`): "Password is too long (at
   most 72 bytes). Accented letters and emoji count as more than one character, so use a shorter one."
3. Every request that carries a password uses it: register, login, and later password reset / change (0007). The
   minimum stays a character count (`min=8`).
4. Any other limit that is really in bytes uses `maxbytes=N` too.

## Why
`max=72` let 40 accented letters through (40 characters, 80 bytes); `bcrypt.GenerateFromPassword` then refused
them and register answered 500 — a break of the documented contract ("8–72 characters"). bcrypt's compare also
silently ignores everything past byte 72, so allowing longer input would mean part of a password is never
checked. Validating in the request answers before any service runs and in the same shape as every other field
error; catching bcrypt's error in each service instead depends on every new flow remembering to.

## Adopting it in a derivative
Copy `validation/max_bytes.go` (+ its test), register the tag, add the `maxbytes` case to the 422 messages, and
put `maxbytes=72` on every field that is hashed with bcrypt or compared against a bcrypt hash; update `docs/API.md`.
Mapping `bcrypt.ErrPasswordTooLong` to a 422 in the service is a fine second guard, not a replacement.
