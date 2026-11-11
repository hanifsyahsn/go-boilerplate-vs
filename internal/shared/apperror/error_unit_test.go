package apperror

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	underlying := errors.New("db connection failed")

	tests := []struct {
		name    string
		code    Code
		message string
		err     error
	}{
		{"with a wrapped error", CodeInternal, "custom message", underlying},
		{"without a wrapped error", CodeNotFound, "not found", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.code, tt.message, tt.err)
			require.NotNil(t, got)
			assert.Equal(t, tt.code, got.Code)
			assert.Equal(t, tt.message, got.Message)
			assert.Equal(t, tt.err, got.Err)
			assert.Nil(t, got.Source)

			var asErr error = got
			assert.Equal(t, tt.message, asErr.Error())
		})
	}
}

func TestAppError_Unwrap_U(t *testing.T) {
	inner := New(CodeNotFound, "Inner.", context.Canceled)

	testCases := []struct {
		name      string
		err       error
		target    error
		wantIs    bool
		wantCode  Code // the AppError errors.As finds
		wantNoApp bool
	}{
		{name: "Is through the cause", err: New(CodeInternal, "Oops.", context.Canceled), target: context.Canceled, wantIs: true, wantCode: CodeInternal},
		{
			name: "Is through a %w cause", err: New(CodeInternal, "Oops.", fmt.Errorf("query: %w", context.Canceled)),
			target: context.Canceled, wantIs: true, wantCode: CodeInternal,
		},
		{
			name: "Is through a nested AppError", err: New(CodeInternal, "Outer.", fmt.Errorf("wrap: %w", inner)),
			target: context.Canceled, wantIs: true, wantCode: CodeInternal,
		},
		{name: "a different cause", err: New(CodeInternal, "Oops.", errors.New("db down")), target: context.Canceled, wantIs: false, wantCode: CodeInternal},
		{name: "no cause", err: New(CodeNotFound, "Gone.", nil), target: context.Canceled, wantIs: false, wantCode: CodeNotFound},
		{name: "As finds an AppError behind %w", err: fmt.Errorf("svc: %w", inner), target: context.Canceled, wantIs: true, wantCode: CodeNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantIs, errors.Is(tc.err, tc.target))

			var appErr *AppError
			require.ErrorAs(t, tc.err, &appErr)
			require.Equal(t, tc.wantCode, appErr.Code, "the outermost AppError")
		})
	}
}

func TestAppError_WithSource(t *testing.T) {
	tests := []struct {
		name string
		set  func(e *AppError) *AppError
		want Source
	}{
		{"pointer", func(e *AppError) *AppError { return e.WithPointer("/data/type") }, Source{Pointer: "/data/type"}},
		{"parameter", func(e *AppError) *AppError { return e.WithParameter("sort") }, Source{Parameter: "sort"}},
		{"header", func(e *AppError) *AppError { return e.WithHeader("Accept") }, Source{Header: "Accept"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(CodeBadRequest, "bad", nil)
			got := tt.set(e)
			require.Same(t, e, got)
			require.Equal(t, &tt.want, got.Source)
		})
	}
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name string
		errs ValidationErrors
		want string
	}{
		{"none", nil, ""},
		{"one", ValidationErrors{{Detail: "Title is required."}}, "Title is required."},
		{"several joined", ValidationErrors{{Detail: "A."}, {Detail: "B."}}, "A. B."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.errs.Error())
		})
	}
}

func TestFieldPointer(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		want      string
	}{
		{"a top-level field", "Request.title", "/title"},
		{"a nested field", "Request.recipient.email", "/recipient/email"},
		{"a slice element's field", "Request.items[0].id", "/items/0/id"},
		{"a slice element", "Request.ids[2]", "/ids/2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fieldPointer(tt.namespace))
		})
	}
}

// genericCodes is every code apperror declares, with the id, status and title
// the API has always sent for it: changing one is a breaking change for clients.
var genericCodes = []struct {
	name   string
	code   Code
	id     string
	status int
	title  string
}{
	{"internal", CodeInternal, "internal_error", http.StatusInternalServerError, "Internal Server Error"},
	{"not found", CodeNotFound, "not_found", http.StatusNotFound, "Not Found"},
	{"conflict", CodeConflict, "conflict", http.StatusConflict, "Conflict"},
	{"unauthorized", CodeUnauthorized, "unauthorized", http.StatusUnauthorized, "Unauthorized"},
	{"forbidden", CodeForbidden, "forbidden", http.StatusForbidden, "Forbidden"},
	{"bad request", CodeBadRequest, "bad_request", http.StatusBadRequest, "Bad Request"},
	{"token expired", CodeTokenExpired, "token_expired", http.StatusUnauthorized, "Token Expired"},
	{"too many requests", CodeTooManyRequests, "too_many_requests", http.StatusTooManyRequests, "Too Many Requests"},
	{"unprocessable entity", CodeUnprocessableEntity, "unprocessable_entity", http.StatusUnprocessableEntity, "Unprocessable Entity"},
	{"invalid request body", CodeInvalidRequestBody, "invalid_request_body", http.StatusUnprocessableEntity, "Unprocessable Entity"},
	{"method not allowed", CodeMethodNotAllowed, "method_not_allowed", http.StatusMethodNotAllowed, "Method Not Allowed"},
	{"not acceptable", CodeNotAcceptable, "not_acceptable", http.StatusNotAcceptable, "Not Acceptable"},
	{"unsupported media type", CodeUnsupportedMedia, "unsupported_media_type", http.StatusUnsupportedMediaType, "Unsupported Media Type"},
}

func TestGenericCodes_KeepTheirIDStatusAndTitle_U(t *testing.T) {
	for _, tt := range genericCodes {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.id, tt.code.String())
			assert.Equal(t, tt.status, tt.code.Status())
			assert.Equal(t, tt.title, tt.code.Title())
			assert.False(t, tt.code.IsZero())
		})
	}
}

func TestGenericCodes_HaveUniqueIDs_U(t *testing.T) {
	seen := map[string]string{}
	for _, tt := range genericCodes {
		if other, ok := seen[tt.code.String()]; ok {
			t.Fatalf("%s and %s share the id %q", other, tt.name, tt.code.String())
		}
		seen[tt.code.String()] = tt.name
	}
}

func TestNewCode_U(t *testing.T) {
	testCases := []struct {
		name      string
		build     func() Code
		wantID    string
		wantStat  int
		wantTitle string
		wantPanic string
	}{
		{
			name:      "a domain code takes the status it is declared with and its status text as title",
			build:     func() Code { return NewCode("note_archived", http.StatusConflict) },
			wantID:    "note_archived",
			wantStat:  http.StatusConflict,
			wantTitle: "Conflict",
		},
		{
			name:      "WithTitle overrides the title and keeps id and status",
			build:     func() Code { return NewCode("token_gone", http.StatusUnauthorized).WithTitle("Token Gone") },
			wantID:    "token_gone",
			wantStat:  http.StatusUnauthorized,
			wantTitle: "Token Gone",
		},
		{
			name:      "an empty id is refused",
			build:     func() Code { return NewCode("", http.StatusConflict) },
			wantPanic: "apperror.NewCode: empty id",
		},
		{
			name:      "a success status is refused",
			build:     func() Code { return NewCode("ok", http.StatusOK) },
			wantPanic: `apperror.NewCode("ok"): status 200 is not an error status (400–599)`,
		},
		{
			name:      "a status above 599 is refused",
			build:     func() Code { return NewCode("weird", 600) },
			wantPanic: `apperror.NewCode("weird"): status 600 is not an error status (400–599)`,
		},
		{
			name:      "an empty override title is refused",
			build:     func() Code { return NewCode("x", http.StatusConflict).WithTitle("") },
			wantPanic: `apperror.Code("x").WithTitle: empty title`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic != "" {
				require.PanicsWithValue(t, tc.wantPanic, func() { tc.build() })
				return
			}
			c := tc.build()
			assert.Equal(t, tc.wantID, c.String())
			assert.Equal(t, tc.wantStat, c.Status())
			assert.Equal(t, tc.wantTitle, c.Title())
		})
	}
}

func TestCode_Zero_U(t *testing.T) {
	testCases := []struct {
		name string
		code Code
		want bool
	}{
		{"the zero code is zero", Code{}, true},
		{"a declared code is not zero", CodeNotFound, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.code.IsZero())
		})
	}

	t.Run("the zero code answers 500 / Internal Server Error with an empty id", func(t *testing.T) {
		var c Code
		assert.Equal(t, http.StatusInternalServerError, c.Status())
		assert.Equal(t, "Internal Server Error", c.Title())
		assert.Empty(t, c.String())
	})
}

func TestCode_Comparable_U(t *testing.T) {
	testCases := []struct {
		name string
		a, b Code
		want bool
	}{
		{"the same code is equal", CodeConflict, CodeConflict, true},
		{"two codes with the same status are different", CodeConflict, NewCode("note_archived", http.StatusConflict), false},
		{"a re-declared code with the same id and status is equal", NewCode("conflict", http.StatusConflict), CodeConflict, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.a == tc.b)
		})
	}
}

func TestCode_SafeToThrow(t *testing.T) {
	tests := []struct {
		name     string
		code     Code
		expected string
	}{
		{"internal", CodeInternal, "Something went wrong, please try again."},
		{"too many requests", CodeTooManyRequests, "Too many requests."},
		{"unauthorized", CodeUnauthorized, "You're not authorized to perform this action."},
		{"invalid request body", CodeInvalidRequestBody, "We couldn't process your request. Please check your input and try again, or contact support if the problem continues."},
		{"a code without its own message defaults", NewCode("some_domain_code", http.StatusConflict), "Internal Server Error."},
		{"not found defaults (no explicit case)", CodeNotFound, "Internal Server Error."},
		{"bad request defaults (no explicit case)", CodeBadRequest, "Internal Server Error."},
		{"the zero code defaults", Code{}, "Internal Server Error."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.code.SafeToThrow())
		})
	}
}

// helper struct + validator instance to produce real validator.ValidationErrors
type testPayload struct {
	Name     string   `validate:"required"`
	Code     string   `validate:"notblank"`
	Username string   `validate:"alphanum"`
	Password string   `validate:"min=8"`
	Nickname string   `validate:"max=5"`
	Pin      string   `validate:"len=6"`
	Email    string   `validate:"email"`
	Strong   string   `validate:"strongpassword"`
	Unknown  string   `validate:"uppercase"`
	URL      string   `validate:"url"`
	Tags     []string `validate:"unique"`
	IDs      []int64  `validate:"min=1"`
	Refs     []int64  `validate:"max=1"`
	Secret   string   `validate:"maxbytes=4"`
}

func registerCustomValidators(v *validator.Validate) {
	_ = v.RegisterValidation("notblank", func(fl validator.FieldLevel) bool {
		return fl.Field().String() != ""
	})
	_ = v.RegisterValidation("strongpassword", func(fl validator.FieldLevel) bool {
		return false // always fail, we just need the tag to trigger
	})
	_ = v.RegisterValidation("maxbytes", func(fl validator.FieldLevel) bool {
		return false // always fail, we just need the tag to trigger
	})
}

func TestValidatorError(t *testing.T) {
	v := validator.New()
	registerCustomValidators(v)

	payload := testPayload{
		Name:     "",                 // required -> fails
		Code:     "",                 // notblank -> fails
		Username: "abc-123",          // alphanum -> fails
		Password: "short",            // min=8 -> fails
		Nickname: "toolongnick",      // max=5 -> fails
		Pin:      "123",              // len=6 -> fails
		Email:    "not-an-email",     // email -> fails
		Strong:   "whatever",         // strongpassword -> fails (custom always-false)
		Unknown:  "lowercase",        // uppercase -> fails, hits default branch
		URL:      "not a valid url",  // url -> fails (contains spaces, not a valid URI)
		Tags:     []string{"a", "a"}, // unique -> fails
		IDs:      []int64{},          // min=1 on a slice -> fails, counted in items
		Refs:     []int64{1, 2},      // max=1 on a slice -> fails, counted in items
		Secret:   "ééé",              // maxbytes=4 -> fails
	}

	err := v.Struct(payload)
	require.Error(t, err)

	var valErrs validator.ValidationErrors
	require.ErrorAs(t, err, &valErrs)

	results := ValidatorError(valErrs, payload)
	require.Len(t, results, len(valErrs))

	// index results by field for easy assertion regardless of order
	byField := map[string]ValidationError{}
	for _, r := range results {
		byField[r.Field] = r
	}

	assert.Equal(t, "Name is required.", byField["Name"].Detail)
	assert.Equal(t, "Code cannot be blank.", byField["Code"].Detail)
	assert.Equal(t, "Username must contain only letters and numbers.", byField["Username"].Detail)
	assert.Equal(t, "Password must be at least 8 characters long.", byField["Password"].Detail)
	assert.Equal(t, "Nickname must be at most 5 characters long.", byField["Nickname"].Detail)
	assert.Equal(t, "Pin must be exactly 6 characters long.", byField["Pin"].Detail)
	assert.Equal(t, "Email must be a valid email address.", byField["Email"].Detail)
	assert.Equal(t, "Strong must contain at least one uppercase letter, one lowercase letter, one number, and one symbol.", byField["Strong"].Detail)
	assert.Equal(t, "Unknown has an invalid value.", byField["Unknown"].Detail)
	assert.Equal(t, "URL must be a valid URL.", byField["URL"].Detail)
	assert.Equal(t, "Tags must not contain duplicates.", byField["Tags"].Detail)
	assert.Equal(t, "IDs must contain at least 1 item(s).", byField["IDs"].Detail)
	assert.Equal(t, "Refs must contain at most 1 item(s).", byField["Refs"].Detail)
	assert.Equal(t, "Secret is too long (at most 4 bytes). Accented letters and emoji count as more than one character, "+
		"so use a shorter one.", byField["Secret"].Detail)
	assert.Equal(t, "/Name", byField["Name"].Pointer)
}

func TestValidatorError_Small(t *testing.T) {
	type simplePayload struct {
		Email string `validate:"required,email"`
	}

	tests := []struct {
		name    string
		payload *simplePayload // nil => no validation errors at all
		want    []ValidationError
	}{
		{name: "no errors", payload: nil, want: nil},
		{
			name:    "a single required field",
			payload: &simplePayload{},
			want:    []ValidationError{{Field: "Email", Detail: "Email is required.", Pointer: "/Email"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var valErrs validator.ValidationErrors
			if tt.payload != nil {
				require.ErrorAs(t, validator.New().Struct(*tt.payload), &valErrs)
			}
			assert.Equal(t, tt.want, ValidatorError(valErrs, simplePayload{}))
		})
	}
}

func TestFieldLabel(t *testing.T) {
	type Address struct {
		City string `json:"city"`
	}

	type Person struct {
		Name string `json:"name"`
		Home Address
		Age  int
	}

	type LabeledPayload struct {
		Name string `json:"full_name" label:"Custom name"`
	}

	type Item struct {
		MemoID int64 `json:"memo_id" label:"Memo ID"`
	}

	type BulkPayload struct {
		Items    []Item  `json:"items" label:"Items"`
		ItemPtrs []*Item `json:"item_ptrs"`
		IDs      []int64 `json:"ids" label:"Memo IDs"`
		Fixed    [2]Item `json:"fixed"`
	}

	tests := []struct {
		name      string
		obj       interface{}
		namespace string
		expected  string
	}{
		{
			name:      "uses label tag when present",
			obj:       LabeledPayload{},
			namespace: "LabeledPayload.Name",
			expected:  "Custom name",
		},
		{
			name:      "falls back to json tag when no label",
			obj:       Person{},
			namespace: "Person.Name",
			expected:  "name",
		},
		{
			name:      "dereferences pointer struct",
			obj:       &Person{},
			namespace: "Person.Name",
			expected:  "name",
		},
		{
			name:      "traverses nested struct",
			obj:       Person{},
			namespace: "Person.Home.City",
			expected:  "city",
		},
		{
			name:      "field not found falls back to raw part",
			obj:       Person{},
			namespace: "Person.NonExistent",
			expected:  "NonExistent",
		},
		{
			name:      "non-struct field type breaks and falls back to raw part",
			obj:       Person{},
			namespace: "Person.Age.Something",
			expected:  "Something",
		},
		{
			name:      "steps into a slice element's struct and uses its label",
			obj:       BulkPayload{},
			namespace: "BulkPayload.Items[0].MemoID",
			expected:  "Memo ID",
		},
		{
			name:      "steps into a slice of pointers",
			obj:       BulkPayload{},
			namespace: "BulkPayload.ItemPtrs[1].MemoID",
			expected:  "Memo ID",
		},
		{
			name:      "steps into an array element",
			obj:       BulkPayload{},
			namespace: "BulkPayload.Fixed[1].MemoID",
			expected:  "Memo ID",
		},
		{
			name:      "an indexed last part uses the slice field's own label",
			obj:       BulkPayload{},
			namespace: "BulkPayload.IDs[2]",
			expected:  "Memo IDs",
		},
		{
			name:      "the slice itself (no index) uses its label",
			obj:       BulkPayload{},
			namespace: "BulkPayload.Items",
			expected:  "Items",
		},
		{
			name:      "top-level non-struct obj breaks immediately",
			obj:       42,
			namespace: "Foo.Bar",
			expected:  "Bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, fieldLabel(tt.obj, tt.namespace))
		})
	}
}

func TestJSONFieldName(t *testing.T) {
	type Payload struct {
		Named   string `json:"full_name"`
		NoTag   string
		Dashed  string `json:"-"`
		WithOpt string `json:"opt_name,omitempty"`
	}

	rt := reflect.TypeOf(Payload{})

	tests := []struct {
		name     string
		field    string
		expected string
	}{
		{"uses json tag", "Named", "full_name"},
		{"falls back to go field name when no json tag", "NoTag", "NoTag"},
		{"falls back to go field name when json tag is dash", "Dashed", "Dashed"},
		{"strips options from json tag", "WithOpt", "opt_name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, ok := rt.FieldByName(tt.field)
			require.True(t, ok)
			assert.Equal(t, tt.expected, jsonFieldName(field))
		})
	}
}

func TestValidatorError_RequiredWithout_U(t *testing.T) {
	type payload struct {
		Name string `json:"name" validate:"required_without=Ref" label:"Recipient name"`
		Ref  *int64 `json:"ref"`
	}
	ref := int64(1)

	testCases := []struct {
		name    string
		in      payload
		wantErr string // "" => valid
	}{
		{name: "missing while the alternative is missing too", in: payload{}, wantErr: "Recipient name is required."},
		{name: "missing but the alternative is set", in: payload{Ref: &ref}},
		{name: "present", in: payload{Name: "PT X"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.New().Struct(tc.in)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			var ve validator.ValidationErrors
			require.ErrorAs(t, err, &ve)
			got := ValidatorError(ve, tc.in)
			require.Len(t, got, 1)
			require.Equal(t, tc.wantErr, got[0].Detail)
		})
	}
}
