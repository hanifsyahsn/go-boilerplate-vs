package password

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
