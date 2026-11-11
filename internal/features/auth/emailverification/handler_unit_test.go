package emailverification

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

func TestHandler_Verify_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())

	testCases := []struct {
		name       string
		body       string
		stub       func(s *MockS)
		wantCode   int
		wantInBody string
	}{
		{
			name:     "a token: 204 with no body",
			body:     `{"data":{"type":"email-verifications","attributes":{"token":"tok"}}}`,
			stub:     func(s *MockS) { s.EXPECT().Verify(gomock.Any(), Request{Token: "tok"}).Return(nil) },
			wantCode: http.StatusNoContent,
		},
		{
			name: "no token is a 422 on the token", body: `{"data":{"type":"email-verifications","attributes":{}}}`,
			stub: func(*MockS) {}, wantCode: http.StatusUnprocessableEntity, wantInBody: `"pointer":"/data/attributes/token"`,
		},
		{
			name: "a token over 64 characters is a 422",
			body: `{"data":{"type":"email-verifications","attributes":{"token":"` + strings.Repeat("a", 65) + `"}}}`,
			stub: func(*MockS) {}, wantCode: http.StatusUnprocessableEntity, wantInBody: `"pointer":"/data/attributes/token"`,
		},
		{
			name: "a service error is a 500", body: `{"data":{"type":"email-verifications","attributes":{"token":"tok"}}}`,
			stub:     func(s *MockS) { s.EXPECT().Verify(gomock.Any(), gomock.Any()).Return(errors.New("boom")) },
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

			req := httptest.NewRequest(http.MethodPost, "/api/auth/email-verifications", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/vnd.api+json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.wantInBody)
		})
	}
}
