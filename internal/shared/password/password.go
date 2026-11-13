// Package password hashes and checks user passwords (bcrypt), on their NFC
// form (decision 0019).
package password

import (
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"
)

// Normalize returns plain in Unicode NFC, the form every password is hashed
// and compared in — the normalization RFC 8265's OpaqueString profile uses
// (only that step: unassigned code points aren't refused). The same "é" arrives as
// one code point from one keyboard and as "e" + a combining accent from
// another (macOS, some phone keyboards); bcrypt compares bytes, so without
// this a password typed on one device fails on the other.
func Normalize(plain string) string {
	return norm.NFC.String(plain)
}

// FitsNFC reports whether Normalize(plain) is at most n bytes, normalizing one
// segment at a time and stopping as soon as the total passes n — exact, and a
// multi-MB input costs only the first n bytes' worth of work.
func FitsNFC(plain string, n int) bool {
	var it norm.Iter
	it.InitString(norm.NFC, plain)
	total := 0
	for !it.Done() {
		total += len(it.Next())
		if total > n {
			return false
		}
	}
	return true
}

// MaxBytes is bcrypt's input limit. GenerateFromPassword refuses more;
// CompareHashAndPassword does **not** — it silently reads only the first 72
// bytes, so a longer input that starts with the right password would match.
const MaxBytes = 72

// compare is bcrypt's comparison; a seam for the tests that count how many a
// check spends.
var compare = bcrypt.CompareHashAndPassword

// compareFull is compare, but an input over MaxBytes never matches: bcrypt
// would read only its first 72 bytes. Known and unknown emails take this
// path alike (VerifyOrWaste), so skipping the comparison tells nothing.
func compareFull(hash []byte, plain string) bool {
	return len(plain) <= MaxBytes && compare(hash, []byte(plain)) == nil
}

// Hash returns the bcrypt hash of plain's NFC form.
func Hash(plain string) (string, error) {
	return HashCost(plain, bcrypt.DefaultCost)
}

// HashCost is Hash at another bcrypt cost — bcrypt.MinCost keeps test seeding
// fast while hashing exactly the form production hashes.
func HashCost(plain string, cost int) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(Normalize(plain)), cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Match is the outcome of Verify.
type Match struct {
	// OK: the password is right.
	OK bool
	// Legacy: it matched only as sent, not in NFC — a hash made before
	// passwords were normalized — and the NFC form fits bcrypt. The caller
	// should store Hash(plain) in its place, so the next login from a device
	// that sends the other form works. (A legacy password whose NFC form is
	// over 72 bytes — rare, see NFCMaxBytes — still signs in as sent, with
	// Legacy false: it can't be rehashed.)
	Legacy bool
}

// Verify checks plain against hash: its NFC form first, then — only when that
// differs — plain exactly as sent (a hash made before decision 0019). A form
// over MaxBytes never matches. An empty hash (no password set) never matches.
func Verify(hash, plain string) Match {
	if hash == "" {
		return Match{}
	}
	nfc := Normalize(plain)
	if compareFull([]byte(hash), nfc) {
		return Match{OK: true}
	}
	if nfc != plain && compareFull([]byte(hash), plain) {
		return Match{OK: true, Legacy: len(nfc) <= MaxBytes}
	}
	return Match{}
}

// Check reports whether plain matches hash (Verify's OK).
func Check(hash, plain string) bool {
	return Verify(hash, plain).OK
}

// dummyHash is checked against when there's no user / no password, so a
// login answers in about the same time whether or not the account exists.
var dummyHash, _ = Hash("go-boilerplate-timing-equalizer")

// VerifyOrWaste is Verify, but for a missing hash ("" — e.g. an unknown email)
// it still spends the comparisons a wrong password would — one, and a second
// when plain isn't in NFC — and never matches, so the response time doesn't
// tell which emails have an account.
func VerifyOrWaste(hash, plain string) Match {
	if hash == "" {
		Verify(dummyHash, plain)
		return Match{}
	}
	return Verify(hash, plain)
}
