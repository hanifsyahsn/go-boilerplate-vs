package validation

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
)

type testNotBlankStruct struct {
	Name string `validate:"notblank"`
}

func TestNotBlank(t *testing.T) {
	v := validator.New()
	err := v.RegisterValidation("notblank", NotBlank)
	require.NoError(t, err)

	testCases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "valid - normal string",
			value:   "John Doe",
			wantErr: false,
		},
		{
			name:    "invalid - empty string",
			value:   "",
			wantErr: true,
		},
		{
			name:    "invalid - whitespace only",
			value:   "   ",
			wantErr: true,
		},
		{
			name:    "invalid - tab and newline only",
			value:   "\t\n",
			wantErr: true,
		},
		{
			name:    "valid - string with leading/trailing space but has content",
			value:   "  John Doe  ",
			wantErr: false,
		},
		{
			name:    "valid - single character",
			value:   "a",
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err = v.Struct(testNotBlankStruct{Name: tc.value})

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
