package validation

import (
	"strings"

	"github.com/go-playground/validator/v10"
)

func NotBlank(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	return strings.TrimSpace(value) != ""
}
