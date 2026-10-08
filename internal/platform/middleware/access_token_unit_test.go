package middleware

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

func TestAccessTokenMiddleware_U(t *testing.T) {
	claims := jwt.MapClaims{"sub": float64(7), "role": "admin"}

	testCases := []struct {
		name       string
		header     string
		cookie     string
		stub       func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier)
		wantCode   int
		wantInBody string
	}{
		{
			name:   "a bearer token of an active user: id + role in the context",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				q.EXPECT().GetUserIsActive(gomock.Any(), int64(7)).Return(true, nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: "7:admin",
		},
		{
			name:   "the cookie wins over the header",
			header: "Bearer header-token",
			cookie: "cookie-token",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("cookie-token").Return(jwt.MapClaims{"sub": float64(7)}, nil)
				q.EXPECT().GetUserIsActive(gomock.Any(), int64(7)).Return(true, nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: "7:",
		},
		{name: "no token at all", stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {}, wantCode: http.StatusUnauthorized},
		{name: "not a bearer header", header: "Basic abc", stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {}, wantCode: http.StatusUnauthorized},
		{
			name:   "an invalid token",
			header: "Bearer bad",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("bad").Return(nil, errors.New("expired"))
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:   "a deactivated user",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				q.EXPECT().GetUserIsActive(gomock.Any(), int64(7)).Return(false, nil)
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: inactiveAccountMessage,
		},
		{
			name:   "a deleted user",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				q.EXPECT().GetUserIsActive(gomock.Any(), int64(7)).Return(false, sql.ErrNoRows)
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: inactiveAccountMessage,
		},
		{
			name:   "a DB error is a 500",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				q.EXPECT().GetUserIsActive(gomock.Any(), int64(7)).Return(false, sql.ErrConnDone)
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctrl := gomock.NewController(t)
			m := jwttoken.NewMockM(ctrl)
			q := middlewaresqlc.NewMockQuerier(ctrl)
			tc.stub(m, q)

			r := gin.New()
			r.Use(AccessTokenMiddleware(m, q))
			r.GET("/", func(c *gin.Context) {
				c.String(http.StatusOK, "%d:%s", c.GetInt64(internalsharedconstant.UserIdKey), c.GetString(internalsharedconstant.UserRoleKey))
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: internalsharedconstant.AccessTokenCookieKey, Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.wantInBody)
		})
	}
}
