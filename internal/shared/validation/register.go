package validation

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var (
	registerOnce sync.Once
	registerErr  error
)

// RegisterGinValidators wires the custom tags (notblank, rfc3339) into gin's
// binding validator and makes validation errors name fields by their json
// tag. Called once from main; safe to call again (handler tests do).
func RegisterGinValidators() error {
	registerOnce.Do(func() {
		v, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			registerErr = fmt.Errorf("RegisterGinValidators: unexpected validator engine %T", binding.Validator.Engine())
			return
		}

		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})

		if err := v.RegisterValidation("notblank", NotBlank); err != nil {
			registerErr = fmt.Errorf("RegisterValidation(notblank): %w", err)
			return
		}
		if err := v.RegisterValidation("rfc3339", IsRFC3339); err != nil {
			registerErr = fmt.Errorf("RegisterValidation(rfc3339): %w", err)
		}
	})
	return registerErr
}
