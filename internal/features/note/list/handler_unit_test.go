package list

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

func TestHandler_List_U(t *testing.T) {
	notes := []shared.NoteResource{{ID: "5", Title: "Groceries", CreatedBy: shared.UserResource{ID: "7", Name: "Jane"}}}
	meta := pagination.NewMeta(1, 20, 1)

	testCases := []struct {
		name          string
		query         string
		stub          func(s *MockS)
		wantCode      int
		wantInBody    []string
		wantNotInBody []string
	}{
		{
			name: "defaults: newest first, creator included, meta + pagination links",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), Request{Sort: "-created_at"}, int64(7)).Return(notes, meta, nil)
			},
			wantCode: http.StatusOK,
			wantInBody: []string{`"data":[{"type":"notes","id":"5"`, `"included":[{"type":"users","id":"7"`,
				`"total":1`, `"first":"/api/notes?page%5Bnumber%5D=1`, `"next":null`},
		},
		{
			name:  "filter, sort, page and include are passed through",
			query: "?filter%5Bq%5D=+groc+&sort=title&page%5Bnumber%5D=2&page%5Bsize%5D=10&include=",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(),
					Request{Q: "groc", Sort: "title", Request: pagination.Request{Page: 2, PageSize: 10}}, int64(7)).
					Return(notes, meta, nil)
			},
			wantCode:      http.StatusOK,
			wantNotInBody: []string{`"included"`},
		},
		{name: "an unsupported sort field", query: "?sort=body", stub: func(s *MockS) {}, wantCode: http.StatusBadRequest,
			wantInBody: []string{`"source":{"parameter":"sort"}`}},
		{name: "an unsupported filter", query: "?filter%5Btitle%5D=x", stub: func(s *MockS) {}, wantCode: http.StatusBadRequest,
			wantInBody: []string{`"source":{"parameter":"filter[title]"}`}},
		{name: "a bad page size", query: "?page%5Bsize%5D=0", stub: func(s *MockS) {}, wantCode: http.StatusBadRequest},
		{name: "a spec-reserved name it doesn't know", query: "?q=groc", stub: func(s *MockS) {}, wantCode: http.StatusBadRequest},
		{
			name: "service error",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, pagination.Meta{}, errors.New("boom"))
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

			req := httptest.NewRequest(http.MethodGet, "/api/notes"+tc.query, nil)
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
