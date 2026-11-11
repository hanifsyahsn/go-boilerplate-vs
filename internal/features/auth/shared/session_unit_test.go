package shared

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSessionClient_U(t *testing.T) {
	testCases := []struct {
		name          string
		userAgent, ip string
		wantUserAgent string
		wantIP        string
	}{
		{name: "short values are kept as they are", userAgent: "Mozilla/5.0", ip: "10.0.0.1",
			wantUserAgent: "Mozilla/5.0", wantIP: "10.0.0.1"},
		{name: "a user agent longer than 512 characters is cut to 512 runes",
			userAgent: strings.Repeat("é", 600), ip: "10.0.0.1", wantUserAgent: strings.Repeat("é", 512), wantIP: "10.0.0.1"},
		{name: "exactly 512 characters is kept", userAgent: strings.Repeat("a", 512), ip: "",
			wantUserAgent: strings.Repeat("a", 512)},
		{name: "an IP longer than 64 characters is cut to 64", userAgent: "", ip: strings.Repeat("1", 70),
			wantIP: strings.Repeat("1", 64)},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := NewSessionClient(tc.userAgent, tc.ip)
			require.Equal(t, tc.wantUserAgent, got.UserAgent)
			require.Equal(t, tc.wantIP, got.IP)
		})
	}
}
