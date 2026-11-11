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
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestuser"
	"github.com/stretchr/testify/require"
)

func TestAccessTokenMiddleware_U(t *testing.T) {
	sid := uuid.MustParse("6a1f0c9e-3b52-4d8e-9a10-5c2e7f4b8d31")
	claims := jwt.MapClaims{"sub": float64(7), "role": "admin", "sid": sid.String()}
	status := func(q *middlewaresqlc.MockQuerier, row middlewaresqlc.GetAccessStatusRow, err error) {
		q.EXPECT().GetAccessStatus(gomock.Any(), middlewaresqlc.GetAccessStatusParams{SessionID: sid, UserID: 7}).Return(row, err)
	}
	live := middlewaresqlc.GetAccessStatusRow{IsActive: true, SessionLive: true}

	testCases := []struct {
		name       string
		header     string
		cookie     string
		stub       func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier)
		wantCode   int
		wantInBody string
	}{
		{
			name:   "a bearer token of an active user in a live session: id + role + session in the context",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				status(q, live, nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: "7:admin|request ctx:7|session:" + sid.String(),
		},
		{
			name:   "the cookie wins over the header",
			header: "Bearer header-token",
			cookie: "cookie-token",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("cookie-token").Return(jwt.MapClaims{"sub": float64(7), "sid": sid.String()}, nil)
				status(q, live, nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: "7:|request ctx:7|",
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
			// VerifyAccessToken checks sub; this pins the middleware's own guard
			// in case a Maker ever returns claims without one: 401, no DB read.
			name:   "verified claims without a user id are a 401 and the DB isn't read",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(jwt.MapClaims{"role": "admin"}, nil)
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:   "an access token without sid (issued before sessions) is a 401 and the DB isn't read",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(jwt.MapClaims{"sub": float64(7), "role": "admin"}, nil)
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: "You're not authorized to perform this action.",
		},
		{
			name:   "an access token whose sid is not a UUID is a 401",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(jwt.MapClaims{"sub": float64(7), "sid": "abc"}, nil)
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: "You're not authorized to perform this action.",
		},
		{
			name:   "a revoked session is refused at once, before the token expires",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				status(q, middlewaresqlc.GetAccessStatusRow{IsActive: true, SessionLive: false}, nil)
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: "Your session has ended. Please sign in again.",
		},
		{
			name:   "a deactivated user (whatever the session) gets the inactive message",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				status(q, middlewaresqlc.GetAccessStatusRow{IsActive: false, SessionLive: false}, nil)
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: inactiveAccountMessage,
		},
		{
			name:   "a deactivated user",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				status(q, middlewaresqlc.GetAccessStatusRow{IsActive: false, SessionLive: true}, nil)
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: inactiveAccountMessage,
		},
		{
			name:   "a deleted user",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				status(q, middlewaresqlc.GetAccessStatusRow{}, sql.ErrNoRows)
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: inactiveAccountMessage,
		},
		{
			name:   "a DB error is a 500",
			header: "Bearer good",
			stub: func(m *jwttoken.MockM, q *middlewaresqlc.MockQuerier) {
				m.EXPECT().VerifyAccessToken("good").Return(claims, nil)
				status(q, middlewaresqlc.GetAccessStatusRow{}, sql.ErrConnDone)
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
				reqUser, _ := requestuser.FromContext(c.Request.Context())
				session := ""
				if v, ok := c.Get(internalsharedconstant.SessionIDKey); ok {
					session = "session:" + v.(uuid.UUID).String()
				}
				c.String(http.StatusOK, "%d:%s|request ctx:%d|%s", c.GetInt64(internalsharedconstant.UserIDKey),
					c.GetString(internalsharedconstant.UserRoleKey), reqUser, session)
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
