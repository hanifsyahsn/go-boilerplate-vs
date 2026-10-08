package create

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

func TestHandler_Create_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())

	note := shared.NoteResource{ID: "5", Title: "Groceries", CreatedBy: shared.UserResource{ID: "7", Name: "Jane"}}
	doc := func(attrs string) string { return `{"data":{"type":"notes","attributes":` + attrs + `}}` }

	testCases := []struct {
		name          string
		query         string
		body          string
		stub          func(s *MockS)
		wantCode      int
		wantLocation  string
		wantInBody    []string
		wantNotInBody []string
	}{
		{
			name: "created: 201, Location, the note with its creator included by default",
			body: doc(`{"title":"Groceries","body":"milk"}`),
			stub: func(s *MockS) {
				s.EXPECT().Create(gomock.Any(), Request{Title: "Groceries", Body: "milk"}, int64(7)).Return(note, nil)
			},
			wantCode:     http.StatusCreated,
			wantLocation: "/api/notes/5",
			wantInBody: []string{`"jsonapi":{"version":"1.1"}`, `"type":"notes"`, `"id":"5"`,
				`"included":[{"type":"users","id":"7"`, `"links":{"self":"/api/notes"}`},
		},
		{
			name:  "?include= (empty) and a sparse fieldset",
			query: "?include=&fields%5Bnotes%5D=title",
			body:  doc(`{"title":"Groceries"}`),
			stub: func(s *MockS) {
				s.EXPECT().Create(gomock.Any(), Request{Title: "Groceries"}, int64(7)).Return(note, nil)
			},
			wantCode:      http.StatusCreated,
			wantLocation:  "/api/notes/5",
			wantInBody:    []string{`"attributes":{"title":"Groceries"}`},
			wantNotInBody: []string{`"included"`, `"relationships"`},
		},
		{
			name:       "unsupported ?include is a 400 on the parameter, before any write",
			query:      "?include=tags",
			body:       doc(`{"title":"Groceries"}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusBadRequest,
			wantInBody: []string{`Can't include \"tags\"`, `"source":{"parameter":"include"}`},
		},
		{
			name:       "a missing title points at the attribute",
			body:       doc(`{"body":"milk"}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusUnprocessableEntity,
			wantInBody: []string{"Title is required.", `"source":{"pointer":"/data/attributes/title"}`},
		},
		{
			name:       "a blank title",
			body:       doc(`{"title":"   "}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusUnprocessableEntity,
			wantInBody: []string{"Title cannot be blank."},
		},
		{
			name:       "another resource type is a 409",
			body:       `{"data":{"type":"users","attributes":{"title":"x"}}}`,
			stub:       func(s *MockS) {},
			wantCode:   http.StatusConflict,
			wantInBody: []string{`"source":{"pointer":"/data/type"}`},
		},
		{
			name:     "a client-generated id is a 403",
			body:     `{"data":{"type":"notes","id":"9","attributes":{"title":"x"}}}`,
			stub:     func(s *MockS) {},
			wantCode: http.StatusForbidden,
		},
		{
			name:     "plain JSON is not a document: 400",
			body:     `{"title":"Groceries"}`,
			stub:     func(s *MockS) {},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "service app error",
			body: doc(`{"title":"Groceries"}`),
			stub: func(s *MockS) {
				s.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(shared.NoteResource{}, apperror.New(apperror.CodeNotFound, shared.NotFoundMessage, nil))
			},
			wantCode:   http.StatusNotFound,
			wantInBody: []string{shared.NotFoundMessage},
		},
		{
			name: "service generic error",
			body: doc(`{"title":"Groceries"}`),
			stub: func(s *MockS) {
				s.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(shared.NoteResource{}, errors.New("boom"))
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

			req := httptest.NewRequest(http.MethodPost, "/api/notes"+tc.query, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Equal(t, tc.wantLocation, w.Header().Get("Location"))
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}
			for _, s := range tc.wantNotInBody {
				require.NotContains(t, w.Body.String(), s)
			}
		})
	}
}
