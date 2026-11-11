package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestHash_LengthLimit_U pins bcrypt's input limit, in bytes: the reason the
// requests validate passwords with maxbytes=72, not a character count.
func TestHash_LengthLimit_U(t *testing.T) {
	testCases := []struct {
		name    string
		plain   string
		wantErr bool
	}{
		{name: "72 plain letters (72 bytes) hash", plain: strings.Repeat("a", 72)},
		{name: "36 accented letters (72 bytes) hash", plain: strings.Repeat("é", 36)},
		{name: "73 bytes are refused", plain: strings.Repeat("a", 73), wantErr: true},
		{name: "40 accented letters (40 characters, 80 bytes) are refused", plain: strings.Repeat("é", 40), wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := Hash(tc.plain)
			if tc.wantErr {
				require.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
				require.Empty(t, hash)
				return
			}
			require.NoError(t, err)
			require.True(t, Check(hash, tc.plain))
		})
	}
}

func TestHashAndCheck_U(t *testing.T) {
	hash, err := Hash("S3cret!pass")
	require.NoError(t, err)
	require.NotEqual(t, "S3cret!pass", hash)

	testCases := []struct {
		name  string
		check func(hash, plain string) bool
		hash  string
		plain string
		want  bool
	}{
		{name: "matching password", check: Check, hash: hash, plain: "S3cret!pass", want: true},
		{name: "wrong password", check: Check, hash: hash, plain: "wrong", want: false},
		{name: "not a bcrypt hash", check: Check, hash: "plain-text", plain: "plain-text", want: false},
		{name: "an empty hash never matches", check: Check, hash: "", plain: "", want: false},
		{name: "CheckOrWaste - matching password", check: CheckOrWaste, hash: hash, plain: "S3cret!pass", want: true},
		{name: "CheckOrWaste - wrong password", check: CheckOrWaste, hash: hash, plain: "wrong", want: false},
		{name: "CheckOrWaste - no hash still compares (against the dummy), never matches",
			check: CheckOrWaste, hash: "", plain: "go-boilerplate-timing-equalizer", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.check(tc.hash, tc.plain))
		})
	}
}
