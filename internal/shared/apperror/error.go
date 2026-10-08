package apperror

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type Code string

// please create a unique code when client needs a specific logic to handle
const (
	CodeInternal            Code = "internal_error"
	CodeNotFound            Code = "not_found"
	CodeConflict            Code = "conflict"
	CodeUnauthorized        Code = "unauthorized"
	CodeForbidden           Code = "forbidden"
	CodeBadRequest          Code = "bad_request"
	CodeTokenExpired        Code = "token_expired"
	CodeTooManyRequests     Code = "too_many_requests"
	CodeUnprocessableEntity Code = "unprocessable_entity"
	CodeInvalidRequestBody  Code = "invalid_request_body"
	CodeMethodNotAllowed    Code = "method_not_allowed"
	CodeNotAcceptable       Code = "not_acceptable"
	CodeUnsupportedMedia    Code = "unsupported_media_type"
)

type AppError struct {
	Code    Code
	Message string
	Err     error
	// Source points at what caused the error (a JSON:API error's "source"):
	// a body member, a query parameter or a header. nil when it's the request
	// as a whole.
	Source *Source
}

// Source is a JSON:API error object's "source" member. Set exactly one field.
type Source struct {
	// Pointer is a JSON Pointer (RFC 6901) into the request document, e.g.
	// "/data/attributes/title".
	Pointer string `json:"pointer,omitempty"`
	// Parameter is the query parameter, e.g. "filter[q]".
	Parameter string `json:"parameter,omitempty"`
	// Header is the request header, e.g. "Content-Type".
	Header string `json:"header,omitempty"`
}

type ValidationError struct {
	Field  string
	Detail string
	// Pointer is the JSON Pointer of the failing field relative to the
	// validated struct, built from json tag names: "/title", "/items/0/id".
	Pointer string
}

// ValidationErrors is a failed validation of one request: one entry per
// failing field. It's an error so a binder can return it next to
// *AppError; handlerutil.WriteError writes it as a 422 with one error object
// per field.
type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
	details := make([]string, 0, len(v))
	for _, e := range v {
		details = append(details, e.Detail)
	}
	return strings.Join(details, " ")
}

func (e *AppError) Error() string {
	return e.Message
}

func New(code Code, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// WithPointer sets the error's source to a JSON Pointer into the request
// document and returns e.
func (e *AppError) WithPointer(pointer string) *AppError {
	e.Source = &Source{Pointer: pointer}
	return e
}

// WithParameter sets the error's source to a query parameter and returns e.
func (e *AppError) WithParameter(parameter string) *AppError {
	e.Source = &Source{Parameter: parameter}
	return e
}

// WithHeader sets the error's source to a request header and returns e.
func (e *AppError) WithHeader(header string) *AppError {
	e.Source = &Source{Header: header}
	return e
}

func fieldLabel(obj interface{}, namespace string) string {
	// namespace from validator is typically "Request.SignerName" or, for nested structs, "Request.Recipient.Email";
	// a slice element carries its index, e.g. "Request.Items[0].NoteID" (dive into a []struct) or "Request.IDs[2]".
	parts := strings.Split(namespace, ".")[1:]
	t := reflect.TypeOf(obj)

	for i, part := range parts {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			break
		}

		name, indexed := part, false
		if idx := strings.IndexByte(part, '['); idx >= 0 {
			name, indexed = part[:idx], true
		}

		field, ok := t.FieldByName(name)
		if !ok {
			break
		}

		if i == len(parts)-1 {
			if label := field.Tag.Get("label"); label != "" {
				return label
			}
			return jsonFieldName(field)
		}

		t = field.Type
		if indexed && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
			// Step from the slice/array into its element type (a pointer
			// element is dereferenced at the top of the next iteration).
			t = t.Elem()
		}
	}

	return parts[len(parts)-1]
}

// jsonFieldName returns the field name from its json tag, falling back to
// the Go field name if no json tag is present.
func jsonFieldName(field reflect.StructField) string {
	jsonTag := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
	if jsonTag == "" || jsonTag == "-" {
		return field.Name
	}
	return jsonTag
}

func ValidatorError(err validator.ValidationErrors, obj interface{}) []ValidationError {
	var errs []ValidationError

	for _, e := range err {
		label := fieldLabel(obj, e.StructNamespace())

		var detail string
		switch e.Tag() {
		case "required", "required_without":
			detail = fmt.Sprintf("%s is required.", label)
		case "notblank":
			detail = fmt.Sprintf("%s cannot be blank.", label)
		case "alphanum":
			detail = fmt.Sprintf("%s must contain only letters and numbers.", label)
		case "min":
			detail = sizeBound(label, "at least", e.Param(), e.Kind())
		case "max":
			detail = sizeBound(label, "at most", e.Param(), e.Kind())
		case "email":
			detail = fmt.Sprintf("%s must be a valid email address.", label)
		case "len":
			detail = fmt.Sprintf("%s must be exactly %s characters long.", label, e.Param())
		case "strongpassword":
			detail = fmt.Sprintf("%s must contain at least one uppercase letter, one lowercase letter, one number, and one symbol.", label)
		case "url":
			detail = fmt.Sprintf("%s must be a valid URL.", label)
		case "unique":
			detail = fmt.Sprintf("%s must not contain duplicates.", label)
		default:
			detail = fmt.Sprintf("%s has an invalid value.", label)
		}

		errs = append(errs, ValidationError{
			Field:   label,
			Detail:  detail,
			Pointer: fieldPointer(e.Namespace()),
		})
	}

	return errs
}

// fieldPointer turns a validator namespace into a JSON Pointer relative to
// the validated struct: "Request.title" => "/title", "Request.items[0].id"
// => "/items/0/id". The namespace carries json tag names once
// validation.RegisterGinValidators has set the tag-name func.
func fieldPointer(namespace string) string {
	parts := strings.Split(namespace, ".")[1:]
	var b strings.Builder
	for _, part := range parts {
		name, index, indexed := strings.Cut(part, "[")
		b.WriteString("/" + name)
		if indexed {
			b.WriteString("/" + strings.TrimSuffix(index, "]"))
		}
	}
	return b.String()
}

// sizeBound words a min/max failure for the field's kind: a list is counted
// in items, anything else (a string) in characters.
func sizeBound(label, bound, n string, k reflect.Kind) string {
	switch k {
	case reflect.Slice, reflect.Array, reflect.Map:
		return fmt.Sprintf("%s must contain %s %s item(s).", label, bound, n)
	default:
		return fmt.Sprintf("%s must be %s %s characters long.", label, bound, n)
	}
}

func (c Code) Title() string {
	switch c {
	case CodeInternal:
		return "Internal Server Error"
	case CodeNotFound:
		return "Not Found"
	case CodeConflict:
		return "Conflict"
	case CodeUnauthorized:
		return "Unauthorized"
	case CodeBadRequest:
		return "Bad Request"
	case CodeTokenExpired:
		return "Token Expired"
	case CodeTooManyRequests:
		return "Too Many Requests"
	case CodeUnprocessableEntity:
		return "Unprocessable Entity"
	case CodeForbidden:
		return "Forbidden"
	case CodeInvalidRequestBody:
		return "Unprocessable Entity"
	case CodeMethodNotAllowed:
		return "Method Not Allowed"
	case CodeNotAcceptable:
		return "Not Acceptable"
	case CodeUnsupportedMedia:
		return "Unsupported Media Type"
	default:
		return "Internal Server Error"
	}
}

func MapToHTTPStatus(code Code) int {
	switch code {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeTooManyRequests:
		return http.StatusTooManyRequests
	case CodeInternal:
		return http.StatusInternalServerError
	case CodeUnprocessableEntity:
		return http.StatusUnprocessableEntity
	case CodeTokenExpired:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeInvalidRequestBody:
		return http.StatusUnprocessableEntity
	case CodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case CodeNotAcceptable:
		return http.StatusNotAcceptable
	case CodeUnsupportedMedia:
		return http.StatusUnsupportedMediaType
	default:
		return http.StatusInternalServerError
	}
}

func (c Code) SafeToThrow() string {
	switch c {
	case CodeInternal:
		return "Something went wrong, please try again."
	case CodeTooManyRequests:
		return "Too many requests."
	case CodeUnauthorized:
		return "You're not authorized to perform this action."
	case CodeInvalidRequestBody:
		return "We couldn't process your request. Please check your input and try again, or contact support if the problem continues."
	default:
		return "Internal Server Error."
	}
}
