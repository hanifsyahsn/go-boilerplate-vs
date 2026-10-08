package logout

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

func TestHandler_Logout_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())

	testCases := []struct {
		name        string
		cookie      string
		body        string
		stub        func(s *MockS)
		wantCode    int
		wantCleared bool
	}{
		{
			name:        "logged in: the cookie's family revoked, both cookies expired",
			cookie:      "ref-cookie",
			stub:        func(s *MockS) { s.EXPECT().Logout(gomock.Any(), "ref-cookie").Return(nil) },
			wantCode:    http.StatusNoContent,
			wantCleared: true,
		},
		{
			name:        "a token in a sessions document",
			body:        `{"data":{"type":"sessions","attributes":{"refresh_token":"ref-body"}}}`,
			stub:        func(s *MockS) { s.EXPECT().Logout(gomock.Any(), "ref-body").Return(nil) },
			wantCode:    http.StatusNoContent,
			wantCleared: true,
		},
		{
			name:        "no token: still a 204",
			stub:        func(s *MockS) { s.EXPECT().Logout(gomock.Any(), "").Return(nil) },
			wantCode:    http.StatusNoContent,
			wantCleared: true,
		},
		{
			name:        "a revoke error is a 500, the cookies are still cleared",
			cookie:      "ref-cookie",
			stub:        func(s *MockS) { s.EXPECT().Logout(gomock.Any(), "ref-cookie").Return(errors.New("boom")) },
			wantCode:    http.StatusInternalServerError,
			wantCleared: true,
		},
		{
			name:     "a malformed body",
			body:     `{`,
			stub:     func(s *MockS) {},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "another resource type: 409",
			body:     `{"data":{"type":"users"}}`,
			stub:     func(s *MockS) {},
			wantCode: http.StatusConflict,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := NewMockS(ctrl)
			tc.stub(svc)

			gin.SetMode(gin.TestMode)
			r := gin.New()
			Route(r.Group("/api"), NewHandler(svc, authshared.CookieConfig{}))

			req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(tc.body))
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: internalsharedconstant.AccessTokenCookieKey, Value: "a"})
				req.AddCookie(&http.Cookie{Name: internalsharedconstant.RefreshTokenCookieKey, Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())

			cleared := map[string]bool{}
			for _, ck := range w.Result().Cookies() {
				cleared[ck.Name] = ck.Value == "" && ck.MaxAge < 0
			}
			require.Equal(t, tc.wantCleared, cleared[internalsharedconstant.AccessTokenCookieKey])
			require.Equal(t, tc.wantCleared, cleared[internalsharedconstant.RefreshTokenCookieKey])
		})
	}
}
