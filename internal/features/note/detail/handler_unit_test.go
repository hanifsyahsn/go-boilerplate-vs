package detail

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

func TestHandler_Get_U(t *testing.T) {
	note := shared.NoteResource{ID: "5", Title: "Groceries", CreatedBy: shared.UserResource{ID: "7", Name: "Jane"}}

	testCases := []struct {
		name          string
		path          string
		stub          func(s *MockS)
		wantCode      int
		wantInBody    []string
		wantNotInBody []string
	}{
		{
			name:       "found: creator included by default, self link",
			path:       "/api/notes/5",
			stub:       func(s *MockS) { s.EXPECT().Get(gomock.Any(), int64(5), int64(7)).Return(note, nil) },
			wantCode:   http.StatusOK,
			wantInBody: []string{`"id":"5"`, `"included":[{"type":"users"`, `"links":{"self":"/api/notes/5"}`},
		},
		{
			name:          "a sparse fieldset on the included creator",
			path:          "/api/notes/5?fields%5Busers%5D=name",
			stub:          func(s *MockS) { s.EXPECT().Get(gomock.Any(), int64(5), int64(7)).Return(note, nil) },
			wantCode:      http.StatusOK,
			wantInBody:    []string{`"attributes":{"name":"Jane"}`},
			wantNotInBody: []string{`"email"`},
		},
		{name: "a non-numeric id is a 404", path: "/api/notes/abc", stub: func(s *MockS) {}, wantCode: http.StatusNotFound},
		{name: "sort is not supported here", path: "/api/notes/5?sort=title", stub: func(s *MockS) {}, wantCode: http.StatusBadRequest},
		{
			name: "not found",
			path: "/api/notes/5",
			stub: func(s *MockS) {
				s.EXPECT().Get(gomock.Any(), int64(5), int64(7)).
					Return(shared.NoteResource{}, apperror.New(apperror.CodeNotFound, shared.NotFoundMessage, nil))
			},
			wantCode:   http.StatusNotFound,
			wantInBody: []string{shared.NotFoundMessage},
		},
		{
			name: "service generic error",
			path: "/api/notes/5",
			stub: func(s *MockS) {
				s.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(shared.NoteResource{}, errors.New("boom"))
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
			r.Use(func(c *gin.Context) { c.Set(internalsharedconstant.UserIdKey, int64(7)); c.Next() })
			Route(r.Group("/api"), NewHandler(svc))

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))

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
