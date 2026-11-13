package validation

import (
	"reflect"
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
)

// NFCMaxBytes is the `nfcmaxbytes=N` tag, for a **new** password (register,
// reset, change): the string is at most N bytes of UTF-8 in the form it is
// hashed in, password.Normalize (NFC, decision 0019). Counting what was sent
// instead would refuse a password on a device that sends the decomposed form
// ("e" + a combining accent is 3 bytes, "é" is 2) although its stored form
// fits, and would let through the rare text NFC makes longer (U+0958 क़ is 3
// bytes, its NFC form 6), which bcrypt then refuses with a 500. Same failures
// as MaxBytes for a bad param or a non-string field. password.FitsNFC stops
// once the NFC form passes N, so a huge body costs little to refuse.
func NFCMaxBytes(fl validator.FieldLevel) bool {
	n, s, ok := byteLimit(fl)
	return ok && password.FitsNFC(s, n)
}

// CheckMaxBytes is the `checkmaxbytes=N` tag, for a password being **checked**
// against a stored hash (login, the current password on change): it passes
// when the string fits in N bytes as sent **or** in NFC — the two forms
// password.Verify compares. NFC alone would lock out a hash made before 0019
// from a password NFC lengthens; as sent alone, a decomposed password from a
// Mac whose stored NFC form fits.
func CheckMaxBytes(fl validator.FieldLevel) bool {
	n, s, ok := byteLimit(fl)
	return ok && (len(s) <= n || password.FitsNFC(s, n))
}

// byteLimit reads a byte-limit tag's param and string field; ok is false for
// a param that isn't a non-negative integer or a field that isn't a string.
func byteLimit(fl validator.FieldLevel) (n int, s string, ok bool) {
	n, err := strconv.Atoi(fl.Param())
	if err != nil || n < 0 || fl.Field().Kind() != reflect.String {
		return 0, "", false
	}
	return n, fl.Field().String(), true
}
