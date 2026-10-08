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
		hash  string
		plain string
		want  bool
	}{
		{name: "matching password", hash: hash, plain: "S3cret!pass", want: true},
		{name: "wrong password", hash: hash, plain: "wrong", want: false},
		{name: "not a bcrypt hash", hash: "plain-text", plain: "plain-text", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, Check(tc.hash, tc.plain))
		})
	}
}
