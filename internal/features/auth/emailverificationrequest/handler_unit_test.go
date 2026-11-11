package emailverificationrequest

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

func TestHandler_Request_U(t *testing.T) {
	const accepted = `{"jsonapi":{"version":"1.1"},"meta":{"message":"We've sent a new verification link."},` +
		`"links":{"self":"/api/me/email-verification-requests"}}`

	testCases := []struct {
		name     string
		body     string
		stub     func(s *MockS)
		wantCode int
		wantBody string
	}{
		{name: "no body: 202 with the meta message", stub: func(s *MockS) { s.EXPECT().Request(gomock.Any(), int64(7)).Return(nil) },
			wantCode: http.StatusAccepted, wantBody: accepted},
		{name: "an empty document is accepted", body: `{"data":{"type":"email-verification-requests"}}`,
			stub:     func(s *MockS) { s.EXPECT().Request(gomock.Any(), int64(7)).Return(nil) },
			wantCode: http.StatusAccepted, wantBody: accepted},
		{name: "an attribute is a 403 (there are none)", body: `{"data":{"type":"email-verification-requests","attributes":{"email":"x@example.com"}}}`,
			stub: func(*MockS) {}, wantCode: http.StatusForbidden},
		{name: "the wrong type is a 409", body: `{"data":{"type":"users"}}`, stub: func(*MockS) {}, wantCode: http.StatusConflict},
		{name: "a service error is a 500", stub: func(s *MockS) { s.EXPECT().Request(gomock.Any(), int64(7)).Return(errors.New("boom")) },
			wantCode: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctrl := gomock.NewController(t)
			svc := NewMockS(ctrl)
			tc.stub(svc)
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set(internalsharedconstant.UserIDKey, int64(7)); c.Next() })
			Route(r.Group("/api"), NewHandler(svc))

			req := httptest.NewRequest(http.MethodPost, "/api/me/email-verification-requests", strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/vnd.api+json")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			if tc.wantBody != "" {
				require.JSONEq(t, tc.wantBody, w.Body.String())
			}
		})
	}
}
