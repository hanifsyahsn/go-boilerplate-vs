package refresh

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

func TestHandler_Refresh_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())

	ok := Result{Tokens: jwttoken.Pair{
		AccessToken:           "new-acc",
		AccessTokenExpiresAt:  time.Now().Add(time.Hour),
		RefreshToken:          "new-ref",
		RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}}

	// The device the handler passes on, from the request (User-Agent, client IP).
	client := authshared.SessionClient{UserAgent: "Mozilla/5.0 (test)", IP: "203.0.113.7"}

	testCases := []struct {
		name       string
		query      string
		cookie     string
		body       string
		stub       func(s *MockS)
		wantCode   int
		wantInBody []string
		wantCookie bool
		// wantCleared: both auth cookies are expired in the response (a
		// refresh that can never succeed again, decision 0020).
		wantCleared bool
	}{
		{
			name:       "from the cookie: both cookies replaced + both tokens in meta",
			cookie:     "ref-cookie",
			stub:       func(s *MockS) { s.EXPECT().Refresh(gomock.Any(), "ref-cookie", client).Return(ok, nil) },
			wantCode:   http.StatusOK,
			wantInBody: []string{`"jsonapi":{"version":"1.1"}`, `"meta":{"access_token":"new-acc"`, `"refresh_token":"new-ref"`},
			wantCookie: true,
		},
		{
			name:       "from a sessions document",
			body:       `{"data":{"type":"sessions","attributes":{"refresh_token":"ref-body"}}}`,
			stub:       func(s *MockS) { s.EXPECT().Refresh(gomock.Any(), "ref-body", client).Return(ok, nil) },
			wantCode:   http.StatusOK,
			wantCookie: true,
		},
		{
			name:     "the cookie wins over the body",
			cookie:   "ref-cookie",
			body:     `{"data":{"type":"sessions","attributes":{"refresh_token":"ref-body"}}}`,
			stub:     func(s *MockS) { s.EXPECT().Refresh(gomock.Any(), "ref-cookie", client).Return(ok, nil) },
			wantCode: http.StatusOK, wantCookie: true,
		},
		{
			name: "session_expired (no / invalid token): 401 and both cookies cleared",
			stub: func(s *MockS) {
				s.EXPECT().Refresh(gomock.Any(), "", client).Return(Result{}, apperror.New(CodeSessionExpired, sessionExpiredMessage, nil))
			},
			wantCode:    http.StatusUnauthorized,
			wantInBody:  []string{sessionExpiredMessage, `"code":"session_expired"`},
			wantCleared: true,
		},
		{
			name:   "session_ended: 401 and both cookies cleared",
			cookie: "ref",
			stub: func(s *MockS) {
				s.EXPECT().Refresh(gomock.Any(), "ref", client).Return(Result{}, apperror.New(middleware.CodeSessionEnded, middleware.SessionEndedMessage, nil))
			},
			wantCode:    http.StatusUnauthorized,
			wantInBody:  []string{`"code":"session_ended"`},
			wantCleared: true,
		},
		{
			name:   "account_inactive: 401 and both cookies cleared",
			cookie: "ref",
			stub: func(s *MockS) {
				s.EXPECT().Refresh(gomock.Any(), "ref", client).Return(Result{}, apperror.New(middleware.CodeAccountInactive, middleware.AccountInactiveMessage, nil))
			},
			wantCode:    http.StatusUnauthorized,
			wantInBody:  []string{`"code":"account_inactive"`},
			wantCleared: true,
		},
		{
			name:   "refresh_superseded: 401 and no cookie touched (the winning request already set the new ones)",
			cookie: "ref",
			stub: func(s *MockS) {
				s.EXPECT().Refresh(gomock.Any(), "ref", client).Return(Result{}, apperror.New(CodeRefreshSuperseded, supersededMessage, nil))
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: []string{`"code":"refresh_superseded"`},
		},
		{
			name:     "a malformed body",
			body:     `{`,
			stub:     func(s *MockS) {},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "query parameters are not supported",
			query:    "?include=user",
			stub:     func(s *MockS) {},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "service generic error: a 500, no cookie touched",
			cookie:   "ref",
			stub:     func(s *MockS) { s.EXPECT().Refresh(gomock.Any(), "ref", client).Return(Result{}, errors.New("boom")) },
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := NewMockS(ctrl)
			tc.stub(svc)

			r := gin.New()
			Route(r.Group("/api"), NewHandler(svc, authshared.CookieConfig{}))

			req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh"+tc.query, strings.NewReader(tc.body))
			req.Header.Set("User-Agent", client.UserAgent)
			req.RemoteAddr = client.IP + ":5555"
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: internalsharedconstant.RefreshTokenCookieKey, Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}

			cookies := map[string]*http.Cookie{}
			for _, ck := range w.Result().Cookies() {
				cookies[ck.Name] = ck
			}
			access, refresh := cookies[internalsharedconstant.AccessTokenCookieKey], cookies[internalsharedconstant.RefreshTokenCookieKey]
			switch {
			case tc.wantCookie:
				require.Equal(t, "new-acc", access.Value)
				require.Equal(t, "new-ref", refresh.Value)
				require.Equal(t, refresh.MaxAge, access.MaxAge, "the access cookie lives as long as the refresh token")
			case tc.wantCleared:
				require.Len(t, cookies, 2)
				require.Empty(t, access.Value)
				require.Negative(t, access.MaxAge, "access cookie expired")
				require.Empty(t, refresh.Value)
				require.Negative(t, refresh.MaxAge, "refresh cookie expired")
			default:
				require.Empty(t, cookies)
			}
		})
	}
}
