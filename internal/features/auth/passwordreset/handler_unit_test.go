package passwordreset

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

func TestHandler_Reset_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())
	doc := func(token, password string) string {
		return `{"data":{"type":"password-resets","attributes":{"token":"` + token + `","password":"` + password + `"}}}`
	}

	testCases := []struct {
		name       string
		body       string
		stub       func(s *MockS)
		wantCode   int
		wantInBody []string
	}{
		{
			name: "a valid token and password: 204 with no body",
			body: doc("tok", "N3w-Passw0rd"),
			stub: func(s *MockS) {
				s.EXPECT().Reset(gomock.Any(), Request{Token: "tok", Password: "N3w-Passw0rd"}).Return(nil)
			},
			wantCode: http.StatusNoContent,
		},
		{
			name: "a bad token: the service's 422 invalid_token on the token",
			body: doc("tok", "N3w-Passw0rd"),
			stub: func(s *MockS) {
				s.EXPECT().Reset(gomock.Any(), gomock.Any()).Return(apperror.New(authshared.CodeInvalidToken,
					authshared.InvalidTokenMessage, nil).WithPointer("/data/attributes/token"))
			},
			wantCode: http.StatusUnprocessableEntity,
			wantInBody: []string{`"code":"invalid_token"`, `"title":"Unprocessable Entity"`,
				"This link is invalid or has expired. Ask for a new one.", `"pointer":"/data/attributes/token"`},
		},
		{
			name: "a password of 73 bytes (36 two-byte letters + 1) is a 422 before the service",
			body: doc("tok", strings.Repeat("é", 36)+"a"), stub: func(*MockS) {},
			wantCode: http.StatusUnprocessableEntity, wantInBody: []string{`"code":"unprocessable_entity"`, `"pointer":"/data/attributes/password"`, "Password must be at most 72 bytes (accented letters, non-Latin scripts and emoji take more than one byte each)."},
		},
		{
			name: "13 क़ (39 bytes sent, 78 in NFC) are a 422, never reaching bcrypt's 500",
			body: doc("tok", strings.Repeat("\u0958", 13)), stub: func(*MockS) {},
			wantCode: http.StatusUnprocessableEntity, wantInBody: []string{`"code":"unprocessable_entity"`,
				`"pointer":"/data/attributes/password"`, "Password must be at most 72 bytes"},
		},
		{
			name: "a short password is a 422", body: doc("tok", "short"), stub: func(*MockS) {},
			wantCode: http.StatusUnprocessableEntity, wantInBody: []string{`"pointer":"/data/attributes/password"`},
		},
		{
			name: "a token longer than 64 characters is a 422", body: doc(strings.Repeat("a", 65), "N3w-Passw0rd"), stub: func(*MockS) {},
			wantCode: http.StatusUnprocessableEntity, wantInBody: []string{`"pointer":"/data/attributes/token"`},
		},
		{
			name: "no token is a 422", body: `{"data":{"type":"password-resets","attributes":{"password":"N3w-Passw0rd"}}}`,
			stub: func(*MockS) {}, wantCode: http.StatusUnprocessableEntity, wantInBody: []string{`"pointer":"/data/attributes/token"`},
		},
		{
			name: "a service error is a 500", body: doc("tok", "N3w-Passw0rd"),
			stub:     func(s *MockS) { s.EXPECT().Reset(gomock.Any(), gomock.Any()).Return(errors.New("boom")) },
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
			Route(r.Group("/api"), NewHandler(svc))

			req := httptest.NewRequest(http.MethodPost, "/api/auth/password-resets", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/vnd.api+json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			if tc.wantCode == http.StatusNoContent {
				require.Empty(t, w.Body.String())
			}
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}
		})
	}
}
