package constant

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsNoBrowserOrigins_U(t *testing.T) {
	testCases := []struct {
		name  string
		entry string
		want  bool
	}{
		{name: "none is the no-browser value", entry: "none", want: true},
		{name: "any case and surrounding spaces still read as none", entry: "  NoNe ", want: true},
		{name: "an origin is not none", entry: "https://none.example.com", want: false},
		{name: "an empty entry is not none", entry: "", want: false},
		{name: "none with extra text is not none", entry: "none2", want: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, IsNoBrowserOrigins(tc.entry))
		})
	}
}
