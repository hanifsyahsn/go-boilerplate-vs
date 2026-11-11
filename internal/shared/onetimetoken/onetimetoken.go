// Package onetimetoken makes the single-use tokens sent in email links
// (password reset, email verification). Only the hash is stored, so a
// leaked database doesn't hand out working links.
package onetimetoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// size is the token's entropy in bytes: 256 bits, base64url = 43 characters.
const size = 32

// New returns a random token for a link (32 bytes, base64url without
// padding, 43 characters) and its hash. err is always nil; it stays in the
// signature so a source that can fail fits.
func New() (plain, hash string, err error) {
	b := make([]byte, size)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error (Go 1.24+): it crashes the program instead
	plain = base64.RawURLEncoding.EncodeToString(b)
	return plain, Hash(plain), nil
}

// Hash is the hex SHA-256 of plain; only the hash is stored. A plain SHA-256
// is enough: the token is 256 random bits, not a password to brute-force.
func Hash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
