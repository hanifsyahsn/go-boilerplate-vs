package me

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

func TestHandler_Get_U(t *testing.T) {
	testCases := []struct {
		name          string
		query         string
		stub          func(s *MockS)
		wantCode      int
		wantInBody    []string
		wantNotInBody []string
	}{
		{
			name: "the caller's profile as a users resource",
			stub: func(s *MockS) {
				s.EXPECT().Get(gomock.Any(), int64(7)).Return(MeResource{ID: "7", Name: "Jane", Role: "admin"}, nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: []string{`"type":"users"`, `"id":"7"`, `"role":"admin"`, `"links":{"self":"/api/me"}`},
		},
		{
			name:  "a sparse fieldset",
			query: "?fields%5Busers%5D=name",
			stub: func(s *MockS) {
				s.EXPECT().Get(gomock.Any(), int64(7)).Return(MeResource{ID: "7", Name: "Jane", Role: "admin"}, nil)
			},
			wantCode:      http.StatusOK,
			wantInBody:    []string{`"attributes":{"name":"Jane"}`},
			wantNotInBody: []string{`"role"`},
		},
		{
			name:     "include is not supported",
			query:    "?include=notes",
			stub:     func(s *MockS) {},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "service app error",
			stub: func(s *MockS) {
				s.EXPECT().Get(gomock.Any(), int64(7)).Return(MeResource{}, apperror.New(apperror.CodeNotFound, "Your account was not found.", nil))
			},
			wantCode:   http.StatusNotFound,
			wantInBody: []string{"Your account was not found."},
		},
		{
			name:     "service generic error",
			stub:     func(s *MockS) { s.EXPECT().Get(gomock.Any(), int64(7)).Return(MeResource{}, errors.New("boom")) },
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := NewMockS(ctrl)
			tc.stub(svc)

			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set(internalsharedconstant.UserIdKey, int64(7)); c.Next() })
			Route(r.Group("/api"), NewHandler(svc))

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/me"+tc.query, nil))

			require.Equal(t, tc.wantCode, w.Code)
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}
			for _, s := range tc.wantNotInBody {
				require.NotContains(t, w.Body.String(), s)
			}
		})
	}
}
