package apperror

import (
	"errors"
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

func TestCode_Title(t *testing.T) {
	tests := []struct {
		name     string
		code     Code
		expected string
	}{
		{"internal", CodeInternal, "Internal Server Error"},
		{"not found", CodeNotFound, "Not Found"},
		{"conflict", CodeConflict, "Conflict"},
		{"unauthorized", CodeUnauthorized, "Unauthorized"},
		{"bad request", CodeBadRequest, "Bad Request"},
		{"token expired", CodeTokenExpired, "Token Expired"},
		{"too many requests", CodeTooManyRequests, "Too Many Requests"},
		{"unprocessable entity", CodeUnprocessableEntity, "Unprocessable Entity"},
		{"forbidden", CodeForbidden, "Forbidden"},
		{"method not allowed", CodeMethodNotAllowed, "Method Not Allowed"},
		{"not acceptable", CodeNotAcceptable, "Not Acceptable"},
		{"unsupported media type", CodeUnsupportedMedia, "Unsupported Media Type"},
		{"unknown code defaults to internal", Code("some_unknown_code"), "Internal Server Error"},
		{"empty code defaults to internal", Code(""), "Internal Server Error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.code.Title())
		})
	}
}

func TestMapToHTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		code     Code
		expected int
	}{
		{"not found", CodeNotFound, http.StatusNotFound},
		{"conflict", CodeConflict, http.StatusConflict},
		{"unauthorized", CodeUnauthorized, http.StatusUnauthorized},
		{"bad request", CodeBadRequest, http.StatusBadRequest},
		{"too many requests", CodeTooManyRequests, http.StatusTooManyRequests},
		{"internal", CodeInternal, http.StatusInternalServerError},
		{"unprocessable", CodeUnprocessableEntity, http.StatusUnprocessableEntity},
		{"token expired", CodeTokenExpired, http.StatusUnauthorized},
		{"forbidden", CodeForbidden, http.StatusForbidden},
		{"method not allowed", CodeMethodNotAllowed, http.StatusMethodNotAllowed},
		{"not acceptable", CodeNotAcceptable, http.StatusNotAcceptable},
		{"unsupported media type", CodeUnsupportedMedia, http.StatusUnsupportedMediaType},
		{"unknown code defaults to 500", Code("some_unknown_code"), http.StatusInternalServerError},
		{"empty code defaults to 500", Code(""), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, MapToHTTPStatus(tt.code))
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
		{"unknown code defaults", Code("some_unknown_code"), "Internal Server Error."},
		{"not found defaults (no explicit case)", CodeNotFound, "Internal Server Error."},
		{"bad request defaults (no explicit case)", CodeBadRequest, "Internal Server Error."},
		{"empty code defaults", Code(""), "Internal Server Error."},
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
}

func registerCustomValidators(v *validator.Validate) {
	_ = v.RegisterValidation("notblank", func(fl validator.FieldLevel) bool {
		return fl.Field().String() != ""
	})
	_ = v.RegisterValidation("strongpassword", func(fl validator.FieldLevel) bool {
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
	}

	err := v.Struct(payload)
	require.Error(t, err)

	valErrs, ok := err.(validator.ValidationErrors)
	require.True(t, ok)

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
