package validation

import (
	"github.com/go-playground/validator/v10"
)

// MaxBytes is the `maxbytes=N` tag: the string is at most N bytes of UTF-8.
// The built-in `max` counts characters (runes), which is wrong for a limit
// that is really in bytes — bcrypt refuses a password over 72 bytes, and 40
// accented letters are 40 characters but 80 bytes. A param that isn't a
// non-negative integer, or a field that isn't a string, is a programming error
// and fails every value (reflect's String() of another kind is a description
// like "<[]uint8 Value>", which would pass by accident).
func MaxBytes(fl validator.FieldLevel) bool {
	n, s, ok := byteLimit(fl)
	return ok && len(s) <= n
}
