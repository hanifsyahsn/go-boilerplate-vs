package delete

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

func TestHandler_Delete_U(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		stub     func(s *MockS)
		wantCode int
	}{
		{name: "deleted: 204", path: "/api/notes/5", stub: func(s *MockS) { s.EXPECT().Delete(gomock.Any(), int64(5), int64(7)).Return(nil) }, wantCode: http.StatusNoContent},
		{name: "a bad id is a 404", path: "/api/notes/x", stub: func(s *MockS) {}, wantCode: http.StatusNotFound},
		{name: "no query parameters are supported", path: "/api/notes/5?include=created_by", stub: func(s *MockS) {}, wantCode: http.StatusBadRequest},
		{
			name: "not found",
			path: "/api/notes/5",
			stub: func(s *MockS) {
				s.EXPECT().Delete(gomock.Any(), int64(5), int64(7)).Return(apperror.New(apperror.CodeNotFound, shared.NotFoundMessage, nil))
			},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "service generic error",
			path:     "/api/notes/5",
			stub:     func(s *MockS) { s.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom")) },
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
			r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, tc.path, nil))

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			if tc.wantCode == http.StatusNoContent {
				require.Empty(t, w.Body.String())
			}
		})
	}
}
