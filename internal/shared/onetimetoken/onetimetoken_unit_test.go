package onetimetoken

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_U(t *testing.T) {
	testCases := []struct {
		name  string
		check func(t *testing.T)
	}{
		{name: "a token is 43 base64url characters and its hash is the hex SHA-256 of it", check: func(t *testing.T) {
			plain, hash, err := New()
			require.NoError(t, err)
			require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, plain)
			require.Equal(t, Hash(plain), hash)
			require.Regexp(t, `^[0-9a-f]{64}$`, hash)
		}},
		{name: "two tokens are never the same", check: func(t *testing.T) {
			a, _, err := New()
			require.NoError(t, err)
			b, _, err := New()
			require.NoError(t, err)
			require.NotEqual(t, a, b)
		}},
		{name: "the hash of a known value", check: func(t *testing.T) {
			require.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", Hash("hello"))
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.check)
	}
}
