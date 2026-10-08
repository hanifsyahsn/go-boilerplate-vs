package validation

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
)

func TestIsRFC3339(t *testing.T) {
	validate := validator.New()
	err := validate.RegisterValidation("rfc3339", IsRFC3339)
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid UTC with Z",
			input:   "2026-08-21T10:15:30Z",
			wantErr: false,
		},
		{
			name:    "valid with timezone offset",
			input:   "2026-08-21T10:15:30+07:00",
			wantErr: false,
		},
		{
			name:    "valid with fractional seconds",
			input:   "2026-08-21T10:15:30.123456789Z",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "date only, no time",
			input:   "2026-08-21",
			wantErr: true,
		},
		{
			name:    "missing timezone",
			input:   "2026-08-21T10:15:30",
			wantErr: true,
		},
		{
			name:    "invalid month",
			input:   "2026-13-21T10:15:30Z",
			wantErr: true,
		},
		{
			name:    "invalid hour",
			input:   "2026-08-21T25:15:30Z",
			wantErr: true,
		},
		{
			name:    "wrong separator (space instead of T)",
			input:   "2026-08-21 10:15:30Z",
			wantErr: true,
		},
		{
			name:    "garbage string",
			input:   "not-a-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Var(tt.input, "rfc3339")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
