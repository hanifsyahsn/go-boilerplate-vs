package passwordresetrequest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

func TestHandler_Request_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())
	const accepted = `{"jsonapi":{"version":"1.1"},"meta":{"message":"If an account exists for that email, we've sent a link to reset the password."},` +
		`"links":{"self":"/api/auth/password-reset-requests"}}`

	testCases := []struct {
		name       string
		body       string
		stub       func(s *MockS)
		wantCode   int
		wantBody   string // exact, when set
		wantInBody string
		query      string // appended to the path
	}{
		{
			name: "a valid request: 202 with the meta-only answer",
			body: `{"data":{"type":"password-reset-requests","attributes":{"email":"jane@example.com"}}}`,
			stub: func(s *MockS) {
				s.EXPECT().Request(gomock.Any(), Request{Email: "jane@example.com"})
			},
			wantCode: http.StatusAccepted, wantBody: accepted,
		},
		{
			name: "not an email: 422 on the attribute", body: `{"data":{"type":"password-reset-requests","attributes":{"email":"jane"}}}`,
			stub: func(*MockS) {}, wantCode: http.StatusUnprocessableEntity, wantInBody: `"pointer":"/data/attributes/email"`,
		},
		{
			name: "the wrong type: 409", body: `{"data":{"type":"users","attributes":{"email":"jane@example.com"}}}`,
			stub: func(*MockS) {}, wantCode: http.StatusConflict,
		},
		{
			name: "a query parameter the endpoint doesn't take: 400, nothing started", query: "?sort=email",
			body: `{"data":{"type":"password-reset-requests","attributes":{"email":"jane@example.com"}}}`,
			stub: func(*MockS) {}, wantCode: http.StatusBadRequest, wantInBody: `"parameter":"sort"`,
		},
		{
			name: "an id is a 403 (no client ids)", body: `{"data":{"type":"password-reset-requests","id":"1","attributes":{"email":"jane@example.com"}}}`,
			stub: func(*MockS) {}, wantCode: http.StatusForbidden,
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

			req := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset-requests"+tc.query, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/vnd.api+json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			if tc.wantBody != "" {
				require.JSONEq(t, tc.wantBody, w.Body.String())
			}
			require.Contains(t, w.Body.String(), tc.wantInBody)
		})
	}
}
