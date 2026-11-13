# 0019 — Passwords are hashed and compared in Unicode NFC

Status: built (2026-11-13)

## Decision
1. Every password is turned into **Unicode NFC** (`password.Normalize`, `golang.org/x/text/unicode/norm`) before
   it is hashed (`password.Hash`) or compared (`password.Verify`). NFC, not NFKC: it only joins what is the same
   letter written two ways ("e" + a combining accent → "é"); NFKC would also fold different characters into one
   (the "ﬁ" ligature → "fi", a full-width "２" → "2"). RFC 8265's OpaqueString profile (passwords) uses NFC.
2. The 72-byte limit (0018) counts the **NFC form** for a **new** password (register, reset, change's new one):
   `nfcmaxbytes=72` (`validation.NFCMaxBytes`, via `password.Normalize`). A password **checked** against a stored
   hash (login, change's current one) uses `checkmaxbytes=72` (`validation.CheckMaxBytes`): it fits as sent **or**
   in NFC — the two forms `Verify` compares, so neither a Mac's decomposed password nor a legacy one NFC makes
   longer is refused before the check. Same 422 text as `maxbytes`, which stays for other byte-limited fields.
3. **Hashes made before this decision** are bcrypt of the bytes as sent. `password.Verify` compares the NFC form
   first and, only when it differs from what was sent, the bytes as sent; a match that way is `Legacy` (when the
   NFC form fits bcrypt; else it signs in as sent and stays as it is). **A form over 72 bytes never matches**
   (`password.MaxBytes`): `bcrypt.CompareHashAndPassword` doesn't check the length and reads only the first 72
   bytes, so without this a legacy 72-byte password plus any extra character would sign in (and the rehash would
   then store the wrong password). After the active check, login stores `password.Hash(plain)` in its place on
   `context.WithoutCancel` (`RehashPassword`: `UPDATE … WHERE id = $1 AND password_hash = <the hash it
   checked>`, so a password changed in between is never overwritten — 0 rows, info `login: rehash skipped,
   stored hash no longer the one checked`). A failed rehash is logged (warn `login: rehash password failed`, with
   `target_user_id`) and never fails the login; the next one tries again. Password change's current-password
   check accepts a legacy match too (the new hash replaces it anyway).
4. **Timing**: `password.VerifyOrWaste` (login) spends the comparisons a wrong password would — one, and a
   second when the input isn't NFC — also for an unknown email, so the extra comparison doesn't reveal which
   emails have an account.
5. The minimum (`min=8`) still counts characters (runes) **as sent**. A decomposed "é" is two runes, so a
   password of 4 decomposed letters passes `min=8` though it is 4 letters in NFC. Accepted: the minimum is a
   nudge, not the security boundary (the failure limits are), and counting it in NFC would refuse what a user
   typed on one device and accept it on another.

## Why
The same "é" arrives as one code point (U+00E9, 2 bytes) from one keyboard and as "e" + U+0301 (3 bytes) from
another — macOS and some phone keyboards send the decomposed form. bcrypt compares bytes, so a user who
registered on one device couldn't sign in on the other, with no way to tell why. Counting the limit as sent had
two more faults: a decomposed password that fits once stored was refused (36 decomposed "é" = 108 bytes sent, 72
stored), and the few characters NFC makes longer (U+0958 "क़", 3 bytes, NFC 6) passed the check and hit bcrypt's
error, a 500.

A legacy hash of the decomposed bytes can't be matched from the composed form until it is rehashed: its owner
signs in once from the device they registered on (or resets the password), and from then on both work.

## Adopting it in a derivative
Copy `password/password.go` (`Normalize`, `Verify` / `Match`, `VerifyOrWaste`, the `compare` seam) and its
tests, `validation/nfc_max_bytes.go` (+ test, register both tags, the 422 case next to `maxbytes`), the login
query `RehashPassword` and the rehash after a `Legacy` match; switch new-password fields from `maxbytes=72` to
`nfcmaxbytes=72` and checked ones (login, current password) to `checkmaxbytes=72`. `SeedUser`-style test
helpers hash the NFC form too. A repo **with existing users** needs step 3 (otherwise users whose device sent
the decomposed form are locked out); a new repo can still take it, it costs nothing. Every place that hashes or compares a
password goes through the `password` package, never bcrypt directly.
