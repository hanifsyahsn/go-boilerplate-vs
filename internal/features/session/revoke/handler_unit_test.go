package revoke

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

func TestHandler_Revoke_U(t *testing.T) {
	current := uuid.MustParse("6a1f0c9e-3b52-4d8e-9a10-5c2e7f4b8d31")
	other := uuid.MustParse("0b8e2c4d-1f3a-4e5b-8c7d-9a0b1c2d3e4f")

	testCases := []struct {
		name        string
		id          string
		query       string
		stub        func(s *MockS)
		wantCode    int
		wantCleared bool // the auth cookies are cleared (a logout)
		wantInBody  string
	}{
		{
			name:     "signing another device out: 204, this device's cookies kept",
			id:       other.String(),
			stub:     func(s *MockS) { s.EXPECT().Revoke(gomock.Any(), int64(7), other.String()).Return(nil) },
			wantCode: http.StatusNoContent,
		},
		{
			name:     "signing the current device out is a logout: 204 and the cookies cleared",
			id:       current.String(),
			stub:     func(s *MockS) { s.EXPECT().Revoke(gomock.Any(), int64(7), current.String()).Return(nil) },
			wantCode: http.StatusNoContent, wantCleared: true,
		},
		{
			name: "a session that isn't the caller's: the service's 404, cookies kept",
			id:   current.String(),
			stub: func(s *MockS) {
				s.EXPECT().Revoke(gomock.Any(), int64(7), current.String()).
					Return(apperror.New(apperror.CodeNotFound, shared.NotFoundMessage, nil))
			},
			wantCode: http.StatusNotFound, wantInBody: "That session doesn't exist.",
		},
		{name: "a query parameter is a 400", id: other.String(), query: "?include=user", stub: func(*MockS) {}, wantCode: http.StatusBadRequest},
		{
			name:     "a plain service error is a 500",
			id:       other.String(),
			stub:     func(s *MockS) { s.EXPECT().Revoke(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom")) },
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctrl := gomock.NewController(t)
			svc := NewMockS(ctrl)
			tc.stub(svc)

			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(internalsharedconstant.UserIDKey, int64(7))
				c.Set(internalsharedconstant.SessionIDKey, current)
				c.Next()
			})
			Route(r.Group("/api"), NewHandler(svc, authshared.CookieConfig{}))

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/me/sessions/"+tc.id+tc.query, nil))

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.wantInBody)
			cleared := map[string]bool{}
			for _, c := range w.Result().Cookies() {
				if c.MaxAge < 0 {
					cleared[c.Name] = true
				}
			}
			require.Equal(t, tc.wantCleared, cleared[internalsharedconstant.AccessTokenCookieKey])
			require.Equal(t, tc.wantCleared, cleared[internalsharedconstant.RefreshTokenCookieKey])
		})
	}
}
