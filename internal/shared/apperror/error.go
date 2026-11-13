package apperror

import (
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

// Code identifies a kind of error for the client (a JSON:API error's "code") and
// carries its HTTP status and title, so a code can't exist without them: a code
// is made only with NewCode, at package level next to the logic that returns it.
//
//	var CodeNoteArchived = apperror.NewCode("note_archived", http.StatusConflict)
//
// The zero Code is an unknown error: 500 / "Internal Server Error". Codes are
// comparable (==, switch) and safe to share.
type Code struct {
	id     string
	status int
	title  string
}

// NewCode makes a code with its HTTP status; the title is http.StatusText(status)
// unless WithTitle overrides it. It panics on an empty id or a status outside
// 400–599: codes are declared in package-level vars, so a bad one stops the
// program at startup.
func NewCode(id string, status int) Code {
	if id == "" {
		panic("apperror.NewCode: empty id")
	}
	if status < 400 || status > 599 {
		panic(fmt.Sprintf("apperror.NewCode(%q): status %d is not an error status (400–599)", id, status))
	}
	return Code{id: id, status: status, title: http.StatusText(status)}
}

// WithTitle returns a copy of c with its own title, for a code whose title isn't
// its status text (token_expired → 401 "Token Expired").
func (c Code) WithTitle(title string) Code {
	if title == "" {
		panic(fmt.Sprintf("apperror.Code(%q).WithTitle: empty title", c.id))
	}
	c.title = title
	return c
}

// String is the code's id, the JSON:API error's "code" member.
func (c Code) String() string { return c.id }

// IsZero reports whether c is the zero Code (no code).
func (c Code) IsZero() bool { return c == Code{} }

// Status is the code's HTTP status; 500 for the zero Code.
func (c Code) Status() int {
	if c.status == 0 {
		return http.StatusInternalServerError
	}
	return c.status
}

// Title is the code's title (a JSON:API error's "title"); "Internal Server
// Error" for the zero Code.
func (c Code) Title() string {
	if c.title == "" {
		return http.StatusText(http.StatusInternalServerError)
	}
	return c.title
}

// The generic codes. A domain declares its own the same way, in its own package.
// Please create a unique code when the client needs a specific logic to handle.
var (
	CodeInternal            = NewCode("internal_error", http.StatusInternalServerError)
	CodeNotFound            = NewCode("not_found", http.StatusNotFound)
	CodeConflict            = NewCode("conflict", http.StatusConflict)
	CodeUnauthorized        = NewCode("unauthorized", http.StatusUnauthorized)
	CodeForbidden           = NewCode("forbidden", http.StatusForbidden)
	CodeBadRequest          = NewCode("bad_request", http.StatusBadRequest)
	CodeTokenExpired        = NewCode("token_expired", http.StatusUnauthorized).WithTitle("Token Expired")
	CodeTooManyRequests     = NewCode("too_many_requests", http.StatusTooManyRequests)
	CodeUnprocessableEntity = NewCode("unprocessable_entity", http.StatusUnprocessableEntity)
	CodeInvalidRequestBody  = NewCode("invalid_request_body", http.StatusUnprocessableEntity)
	CodeMethodNotAllowed    = NewCode("method_not_allowed", http.StatusMethodNotAllowed)
	CodeNotAcceptable       = NewCode("not_acceptable", http.StatusNotAcceptable)
	CodeUnsupportedMedia    = NewCode("unsupported_media_type", http.StatusUnsupportedMediaType)
)

type AppError struct {
	Code    Code
	Message string
	Err     error
	// Source points at what caused the error (a JSON:API error's "source"):
	// a body member, a query parameter or a header. nil when it's the request
	// as a whole.
	Source *Source
	// RetryAfter, when > 0, is how long the client should wait: the error
	// writer sends it as a Retry-After header (whole seconds, rounded up). Set
	// it on every 429 a rate limit refuses (WithRetryAfter).
	RetryAfter time.Duration
}

// RetryAfterHeader is the response header a refused request's wait goes in.
const RetryAfterHeader = "Retry-After"

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

// Unwrap returns the cause, so errors.Is / errors.As see through an AppError
// (errors.Is(err, context.Canceled)). The cause is only logged: an error
// object carries Code / Message / Source, never Err.
func (e *AppError) Unwrap() error {
	return e.Err
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

// WithRetryAfter sets how long the client should wait before trying again
// (a limiter's Decision.RetryAfter) and returns e. The error writer turns it
// into a Retry-After header.
func (e *AppError) WithRetryAfter(d time.Duration) *AppError {
	e.RetryAfter = d
	return e
}

// RetryAfterSeconds is a Retry-After header value: whole seconds, rounded up,
// at least 1.
func RetryAfterSeconds(d time.Duration) string {
	return strconv.Itoa(max(1, int(math.Ceil(d.Seconds()))))
}

func fieldLabel(obj interface{}, namespace string) string {
	// namespace from validator is typically "Request.SignerName" or, for nested structs, "Request.Recipient.Email";
	// a slice element carries its index, e.g. "Request.Items[0].NoteID" (dive into a []struct) or "Request.IDs[2]".
	parts := strings.Split(namespace, ".")[1:]
	t := reflect.TypeOf(obj)

	for i, part := range parts {
		if t.Kind() == reflect.Pointer {
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
		case "maxbytes":
			detail = fmt.Sprintf("%s must be at most %s bytes (accented letters, non-Latin scripts and emoji "+
				"take more than one byte each).", label, e.Param())
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
