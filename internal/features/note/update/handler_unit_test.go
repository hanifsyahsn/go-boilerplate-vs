package update

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

func TestHandler_Update_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())

	note := shared.NoteResource{ID: "5", Title: "New", CreatedBy: shared.UserResource{ID: "7"}}
	title, body := "New", "text"
	doc := func(attrs string) string { return `{"data":{"type":"notes","id":"5","attributes":` + attrs + `}}` }

	testCases := []struct {
		name       string
		path       string
		body       string
		stub       func(s *MockS)
		wantCode   int
		wantInBody []string
	}{
		{
			name: "both attributes",
			path: "/api/notes/5",
			body: doc(`{"title":"New","body":"text"}`),
			stub: func(s *MockS) {
				s.EXPECT().Update(gomock.Any(), int64(5), Request{Title: &title, Body: &body}, int64(7)).Return(note, nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: []string{`"title":"New"`},
		},
		{
			name: "a partial update: only the title",
			path: "/api/notes/5",
			body: doc(`{"title":"New"}`),
			stub: func(s *MockS) {
				s.EXPECT().Update(gomock.Any(), int64(5), Request{Title: &title}, int64(7)).Return(note, nil)
			},
			wantCode: http.StatusOK,
		},
		{
			name: "no attributes: a no-op that returns the note",
			path: "/api/notes/5",
			body: `{"data":{"type":"notes","id":"5"}}`,
			stub: func(s *MockS) {
				s.EXPECT().Update(gomock.Any(), int64(5), Request{}, int64(7)).Return(note, nil)
			},
			wantCode: http.StatusOK,
		},
		{name: "a bad id is a 404", path: "/api/notes/0", body: doc(`{"title":"New"}`), stub: func(s *MockS) {}, wantCode: http.StatusNotFound},
		{
			name:       "an id that isn't the URL's is a 409",
			path:       "/api/notes/5",
			body:       `{"data":{"type":"notes","id":"6","attributes":{"title":"New"}}}`,
			stub:       func(s *MockS) {},
			wantCode:   http.StatusConflict,
			wantInBody: []string{`"source":{"pointer":"/data/id"}`},
		},
		{
			name:       "a blank title is a 422",
			path:       "/api/notes/5",
			body:       doc(`{"title":" "}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusUnprocessableEntity,
			wantInBody: []string{"Title cannot be blank.", `"pointer":"/data/attributes/title"`},
		},
		{
			name:       "a read-only attribute is a 403",
			path:       "/api/notes/5",
			body:       doc(`{"created_at":"2026-01-01T00:00:00Z"}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusForbidden,
			wantInBody: []string{`"pointer":"/data/attributes/created_at"`},
		},
		{
			name: "not found",
			path: "/api/notes/5",
			body: doc(`{"title":"New"}`),
			stub: func(s *MockS) {
				s.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(shared.NoteResource{}, apperror.New(apperror.CodeNotFound, shared.NotFoundMessage, nil))
			},
			wantCode: http.StatusNotFound,
		},
		{
			name: "service generic error",
			path: "/api/notes/5",
			body: doc(`{"title":"New"}`),
			stub: func(s *MockS) {
				s.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(shared.NoteResource{}, errors.New("boom"))
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

			req := httptest.NewRequest(http.MethodPatch, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}
		})
	}
}
