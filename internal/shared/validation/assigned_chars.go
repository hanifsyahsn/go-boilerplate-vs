package validation

import (
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
)

// AssignedChars is the `assignedchars` tag, for a **new** password (register,
// reset, change): every code point is assigned in the Unicode version NFC uses
// (password.AllAssigned). NFC is stable only for assigned code points, so a
// password holding an unassigned one could stop matching its hash after a Go
// upgrade moves x/text to a newer Unicode. Never on a password being checked
// (login, the current one): that would lock its owner out instead. Put it
// after nfcmaxbytes, so a huge body is refused before it is scanned. A field
// that isn't a string fails every value.
func AssignedChars(fl validator.FieldLevel) bool {
	return fl.Field().Kind() == reflect.String && password.AllAssigned(fl.Field().String())
}
