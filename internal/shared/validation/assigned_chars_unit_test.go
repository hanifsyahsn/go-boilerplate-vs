package validation

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
)

func TestAssignedChars_U(t *testing.T) {
	type field struct {
		Value string `validate:"assignedchars"`
	}
	type byteSlice struct {
		Value []byte `validate:"assignedchars"`
	}
	type pointer struct {
		Value *string `validate:"omitempty,assignedchars"`
	}
	ok, bad := "Passw0rd!", "Passw0rd\u0378"

	v := validator.New()
	require.NoError(t, v.RegisterValidation("assignedchars", AssignedChars))

	testCases := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "assigned characters pass", value: field{"Passw0rd \u00e9 \u5bc6\u7801 \u2764\ufe0f"}},
		{name: "an unassigned code point fails", value: field{bad}, wantErr: true},
		{name: "a field that isn't a string fails every value", value: byteSlice{[]byte("a")}, wantErr: true},
		{name: "a *string is checked through the pointer: assigned passes", value: pointer{&ok}},
		{name: "a *string is checked through the pointer: unassigned fails", value: pointer{&bad}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Struct(tc.value)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
