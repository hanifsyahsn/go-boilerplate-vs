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

	testCases := []struct {
		name       string
		query      string
		cookie     string
		body       string
		stub       func(s *MockS)
		wantCode   int
		wantInBody []string
		wantCookie bool
	}{
		{
			name:       "from the cookie: both cookies replaced + both tokens in meta",
			cookie:     "ref-cookie",
			stub:       func(s *MockS) { s.EXPECT().Refresh(gomock.Any(), "ref-cookie").Return(ok, nil) },
			wantCode:   http.StatusOK,
			wantInBody: []string{`"jsonapi":{"version":"1.1"}`, `"meta":{"access_token":"new-acc"`, `"refresh_token":"new-ref"`},
			wantCookie: true,
		},
		{
			name:       "from a sessions document",
			body:       `{"data":{"type":"sessions","attributes":{"refresh_token":"ref-body"}}}`,
			stub:       func(s *MockS) { s.EXPECT().Refresh(gomock.Any(), "ref-body").Return(ok, nil) },
			wantCode:   http.StatusOK,
			wantCookie: true,
		},
		{
			name:     "the cookie wins over the body",
			cookie:   "ref-cookie",
			body:     `{"data":{"type":"sessions","attributes":{"refresh_token":"ref-body"}}}`,
			stub:     func(s *MockS) { s.EXPECT().Refresh(gomock.Any(), "ref-cookie").Return(ok, nil) },
			wantCode: http.StatusOK, wantCookie: true,
		},
		{
			name: "an invalid token is a 401",
			stub: func(s *MockS) {
				s.EXPECT().Refresh(gomock.Any(), "").Return(Result{}, apperror.New(apperror.CodeUnauthorized, sessionExpiredMessage, nil))
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: []string{sessionExpiredMessage},
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
			name:     "service generic error",
			cookie:   "ref",
			stub:     func(s *MockS) { s.EXPECT().Refresh(gomock.Any(), "ref").Return(Result{}, errors.New("boom")) },
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
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: internalsharedconstant.RefreshTokenCookieKey, Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}

			cookies := map[string]string{}
			for _, ck := range w.Result().Cookies() {
				cookies[ck.Name] = ck.Value
			}
			if tc.wantCookie {
				require.Equal(t, "new-acc", cookies[internalsharedconstant.AccessTokenCookieKey])
				require.Equal(t, "new-ref", cookies[internalsharedconstant.RefreshTokenCookieKey])
			} else {
				require.Empty(t, cookies)
			}
		})
	}
}
