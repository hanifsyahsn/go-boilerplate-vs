package login

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

func TestHandler_Login_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())

	exp := time.Now().Add(time.Hour)
	ok := Result{
		User:   authshared.UserResource{ID: "1", Name: "Jane", Email: "jane@example.com", Role: "user"},
		Tokens: jwttoken.Pair{AccessToken: "acc", AccessTokenExpiresAt: exp, RefreshToken: "ref", RefreshTokenExpiresAt: exp},
	}

	doc := func(attrs string) string { return `{"data":{"type":"sessions","attributes":` + attrs + `}}` }

	testCases := []struct {
		name        string
		body        string
		stub        func(s *MockS)
		wantCode    int
		wantInBody  []string
		wantCookies bool
	}{
		{
			name: "logged in: cookies set, the account as data, tokens in meta",
			body: doc(`{"email":"jane@example.com","password":"Passw0rd!"}`),
			stub: func(s *MockS) {
				s.EXPECT().Login(gomock.Any(), Request{Email: "jane@example.com", Password: "Passw0rd!"}).Return(ok, nil)
			},
			wantCode:    http.StatusOK,
			wantInBody:  []string{`"data":{"type":"users","id":"1"`, `"access_token":"acc"`, `"refresh_token":"ref"`},
			wantCookies: true,
		},
		{
			name:       "missing password",
			body:       doc(`{"email":"jane@example.com"}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusUnprocessableEntity,
			wantInBody: []string{"Password is required.", `"pointer":"/data/attributes/password"`},
		},
		{
			name:     "plain JSON is not a document: 400",
			body:     `{"email":"jane@example.com","password":"Passw0rd!"}`,
			stub:     func(s *MockS) {},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "a users document is the wrong type: 409",
			body:     `{"data":{"type":"users","attributes":{"email":"jane@example.com","password":"x"}}}`,
			stub:     func(s *MockS) {},
			wantCode: http.StatusConflict,
		},
		{
			name: "wrong credentials: 401, no cookies",
			body: doc(`{"email":"jane@example.com","password":"wrong"}`),
			stub: func(s *MockS) {
				s.EXPECT().Login(gomock.Any(), gomock.Any()).
					Return(Result{}, apperror.New(apperror.CodeUnauthorized, invalidCredentialsMessage, nil))
			},
			wantCode:   http.StatusUnauthorized,
			wantInBody: []string{invalidCredentialsMessage},
		},
		{
			name:     "service generic error",
			body:     doc(`{"email":"jane@example.com","password":"x"}`),
			stub:     func(s *MockS) { s.EXPECT().Login(gomock.Any(), gomock.Any()).Return(Result{}, errors.New("boom")) },
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

			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(tc.body))
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
			if tc.wantCookies {
				require.Equal(t, "acc", cookies[internalsharedconstant.AccessTokenCookieKey])
				require.Equal(t, "ref", cookies[internalsharedconstant.RefreshTokenCookieKey])
			} else {
				require.Empty(t, cookies)
			}
		})
	}
}
