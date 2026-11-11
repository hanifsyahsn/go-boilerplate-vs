package validation

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
)

func TestMaxBytes_U(t *testing.T) {
	type limited struct {
		Value string `validate:"maxbytes=72"`
	}
	type badParam struct {
		Value string `validate:"maxbytes=abc"`
	}
	type negativeParam struct {
		Value string `validate:"maxbytes=-1"`
	}

	v := validator.New()
	require.NoError(t, v.RegisterValidation("maxbytes", MaxBytes))

	testCases := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "empty passes", value: limited{""}},
		{name: "72 plain letters (72 bytes) pass", value: limited{strings.Repeat("a", 72)}},
		{name: "73 plain letters fail", value: limited{strings.Repeat("a", 73)}, wantErr: true},
		{name: "36 accented letters (72 bytes) pass", value: limited{strings.Repeat("é", 36)}},
		{name: "37 accented letters (74 bytes) fail though only 37 characters", value: limited{strings.Repeat("é", 37)}, wantErr: true},
		{name: "18 emoji (72 bytes) pass", value: limited{strings.Repeat("😀", 18)}},
		{name: "19 emoji (76 bytes) fail", value: limited{strings.Repeat("😀", 19)}, wantErr: true},
		{name: "a param that isn't a number fails every value", value: badParam{"a"}, wantErr: true},
		{name: "a negative param fails every value", value: negativeParam{""}, wantErr: true},
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
