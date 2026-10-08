package shared

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

func TestCookies_U(t *testing.T) {
	cfg := CookieConfig{Secure: true, Domain: "example.com"}
	exp := time.Now().Add(time.Hour)

	testCases := []struct {
		name       string
		set        func(c *gin.Context)
		wantName   string
		wantValue  string
		wantPath   string
		wantDelete bool
	}{
		{
			name:     "access cookie on /",
			set:      func(c *gin.Context) { SetAccessCookie(c, cfg, "a", exp) },
			wantName: internalsharedconstant.AccessTokenCookieKey, wantValue: "a", wantPath: "/",
		},
		{
			name:     "refresh cookie scoped to /api/auth",
			set:      func(c *gin.Context) { SetRefreshCookie(c, cfg, "r", exp) },
			wantName: internalsharedconstant.RefreshTokenCookieKey, wantValue: "r", wantPath: "/api/auth",
		},
		{
			name:     "clear expires the access cookie",
			set:      func(c *gin.Context) { ClearCookies(c, cfg) },
			wantName: internalsharedconstant.AccessTokenCookieKey, wantValue: "", wantPath: "/", wantDelete: true,
		},
		{
			name:     "clear expires the refresh cookie",
			set:      func(c *gin.Context) { ClearCookies(c, cfg) },
			wantName: internalsharedconstant.RefreshTokenCookieKey, wantValue: "", wantPath: "/api/auth", wantDelete: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			tc.set(c)

			var got *http.Cookie
			for _, ck := range w.Result().Cookies() {
				if ck.Name == tc.wantName {
					got = ck
				}
			}
			require.NotNil(t, got)
			require.Equal(t, tc.wantValue, got.Value)
			require.Equal(t, tc.wantPath, got.Path)
			require.Equal(t, "example.com", got.Domain)
			require.True(t, got.HttpOnly)
			require.True(t, got.Secure)
			require.Equal(t, http.SameSiteLaxMode, got.SameSite)
			if tc.wantDelete {
				require.Less(t, got.MaxAge, 0)
			} else {
				require.Greater(t, got.MaxAge, 0)
			}
		})
	}
}
