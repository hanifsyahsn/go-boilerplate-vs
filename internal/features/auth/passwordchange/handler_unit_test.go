package passwordchange

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

func TestHandler_Change_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())
	sid := uuid.New()
	doc := func(current, next string) string {
		return `{"data":{"type":"password-changes","attributes":{"current_password":"` + current + `","password":"` + next + `"}}}`
	}

	testCases := []struct {
		name       string
		body       string
		stub       func(s *MockS)
		wantCode   int
		wantInBody []string
	}{
		{
			name: "a valid change: the caller and their session passed on, 204 with no body",
			body: doc("Passw0rd!", "N3w-Passw0rd"),
			stub: func(s *MockS) {
				s.EXPECT().Change(gomock.Any(), int64(7), sid, Request{CurrentPassword: "Passw0rd!", Password: "N3w-Passw0rd"}).Return(nil)
			},
			wantCode: http.StatusNoContent,
		},
		{
			name: "a wrong current password: the service's 422 on current_password",
			body: doc("nope", "N3w-Passw0rd"),
			stub: func(s *MockS) {
				s.EXPECT().Change(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(apperror.New(CodeWrongPassword,
					"The current password is incorrect.", nil).WithPointer("/data/attributes/current_password"))
			},
			wantCode: http.StatusUnprocessableEntity,
			wantInBody: []string{`"code":"wrong_password"`, "The current password is incorrect.",
				`"pointer":"/data/attributes/current_password"`},
		},
		{
			name: "a new password under 8 characters is a 422 named New password", body: doc("Passw0rd!", "short"), stub: func(*MockS) {},
			wantCode: http.StatusUnprocessableEntity, wantInBody: []string{`"pointer":"/data/attributes/password"`,
				"New password must be at least 8 characters long."},
		},
		{
			name: "a current password over 72 bytes is a 422", body: doc(strings.Repeat("é", 37), "N3w-Passw0rd"), stub: func(*MockS) {},
			wantCode: http.StatusUnprocessableEntity, wantInBody: []string{`"code":"unprocessable_entity"`, `"pointer":"/data/attributes/current_password"`,
				"Current password must be at most 72 bytes (accented letters, non-Latin scripts and emoji take more than one byte each)."},
		},
		{
			name: "a new password over 72 bytes (40 two-byte letters) is a 422 named New password, never reaching bcrypt's 500",
			body: doc("Passw0rd!", strings.Repeat("é", 40)), stub: func(*MockS) {},
			wantCode: http.StatusUnprocessableEntity, wantInBody: []string{`"code":"unprocessable_entity"`, `"pointer":"/data/attributes/password"`,
				"New password must be at most 72 bytes (accented letters, non-Latin scripts and emoji take more than one byte each)."},
		},
		{
			name: "a service error is a 500", body: doc("Passw0rd!", "N3w-Passw0rd"),
			stub: func(s *MockS) {
				s.EXPECT().Change(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom"))
			},
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
				c.Set(internalsharedconstant.SessionIDKey, sid)
				c.Next()
			})
			Route(r.Group("/api"), NewHandler(svc))

			req := httptest.NewRequest(http.MethodPost, "/api/auth/password-changes", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/vnd.api+json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}
		})
	}
}
