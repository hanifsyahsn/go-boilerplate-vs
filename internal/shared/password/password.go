// Package password hashes and checks user passwords (bcrypt).
package password

import "golang.org/x/crypto/bcrypt"

// Hash returns the bcrypt hash of plain.
func Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Check reports whether plain matches hash. An empty hash (no password set)
// never matches.
func Check(hash, plain string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// dummyHash is checked against when there's no user / no password, so a
// login answers in about the same time whether or not the account exists.
var dummyHash, _ = Hash("go-boilerplate-timing-equalizer")

// CheckOrWaste is Check, but for a missing hash ("" — e.g. an unknown email)
// it still spends a bcrypt comparison and returns false, so the response time
// doesn't tell which emails have an account.
func CheckOrWaste(hash, plain string) bool {
	if hash == "" {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(plain))
		return false
	}
	return Check(hash, plain)
}
