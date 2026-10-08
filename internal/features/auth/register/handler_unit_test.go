package register

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

func TestHandler_Register_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())

	doc := func(attrs string) string { return `{"data":{"type":"users","attributes":` + attrs + `}}` }
	valid := doc(`{"name":"Jane","email":"jane@example.com","password":"Passw0rd!"}`)

	testCases := []struct {
		name          string
		body          string
		stub          func(s *MockS)
		wantCode      int
		wantInBody    []string
		wantNotInBody []string
	}{
		{
			name: "created: the account, never the password",
			body: valid,
			stub: func(s *MockS) {
				s.EXPECT().Register(gomock.Any(), Request{Name: "Jane", Email: "jane@example.com", Password: "Passw0rd!"}).
					Return(authshared.UserResource{ID: "1", Name: "Jane", Email: "jane@example.com", Role: "user"}, nil)
			},
			wantCode:      http.StatusCreated,
			wantInBody:    []string{`"type":"users"`, `"role":"user"`},
			wantNotInBody: []string{"Passw0rd!"},
		},
		{
			name:       "an invalid email points at it",
			body:       doc(`{"name":"Jane","email":"nope","password":"Passw0rd!"}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusUnprocessableEntity,
			wantInBody: []string{"Email must be a valid email address.", `"pointer":"/data/attributes/email"`},
		},
		{
			name:       "a short password",
			body:       doc(`{"name":"Jane","email":"jane@example.com","password":"short"}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusUnprocessableEntity,
			wantInBody: []string{"Password must be at least 8 characters long."},
		},
		{
			name:       "the role can't be chosen: 403",
			body:       doc(`{"name":"Jane","email":"jane@example.com","password":"Passw0rd!","role":"admin"}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusForbidden,
			wantInBody: []string{`"pointer":"/data/attributes/role"`},
		},
		{
			name:     "another resource type: 409",
			body:     `{"data":{"type":"sessions","attributes":{}}}`,
			stub:     func(s *MockS) {},
			wantCode: http.StatusConflict,
		},
		{
			name: "a taken email",
			body: valid,
			stub: func(s *MockS) {
				s.EXPECT().Register(gomock.Any(), gomock.Any()).
					Return(authshared.UserResource{}, apperror.New(apperror.CodeConflict, "That email is already registered.", nil))
			},
			wantCode:   http.StatusConflict,
			wantInBody: []string{"That email is already registered."},
		},
		{
			name: "service generic error",
			body: valid,
			stub: func(s *MockS) {
				s.EXPECT().Register(gomock.Any(), gomock.Any()).Return(authshared.UserResource{}, errors.New("boom"))
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := NewMockS(ctrl)
			tc.stub(svc)

			r := gin.New()
			Route(r.Group("/api"), NewHandler(svc))

			req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}
			for _, s := range tc.wantNotInBody {
				require.NotContains(t, w.Body.String(), s)
			}
		})
	}
}
