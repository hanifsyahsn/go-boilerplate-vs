package utility

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTruncateRunes_U(t *testing.T) {
	testCases := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "shorter than n: kept", s: "abc", n: 5, want: "abc"},
		{name: "exactly n: kept", s: "abcde", n: 5, want: "abcde"},
		{name: "longer: cut to n runes, never through a multi-byte character", s: strings.Repeat("é", 10), n: 4, want: "éééé"},
		{name: "n of 0: empty", s: "abc", n: 0, want: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, TruncateRunes(tc.s, tc.n))
		})
	}
}
