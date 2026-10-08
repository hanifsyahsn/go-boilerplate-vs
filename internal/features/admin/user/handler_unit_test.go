package user

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

func TestHandler_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())

	active, inactive := true, false
	doc := func(id, attrs string) string {
		return `{"data":{"type":"users","id":"` + id + `","attributes":` + attrs + `}}`
	}

	testCases := []struct {
		name       string
		method     string
		path       string
		body       string
		stub       func(s *MockS)
		wantCode   int
		wantInBody []string
	}{
		{
			name:   "list with filters, sort and a page",
			method: http.MethodGet,
			path:   "/api/admin/users?filter%5Bq%5D=jane&filter%5Bis_active%5D=true&sort=name&page%5Bnumber%5D=1&page%5Bsize%5D=5",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), ListRequest{Q: "jane", IsActive: &active, Sort: "name", Request: pagination.Request{Page: 1, PageSize: 5}}).
					Return([]UserResource{{ID: "3", Name: "Jane", IsActive: true}}, pagination.NewMeta(1, 5, 1), nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: []string{`"type":"users","id":"3"`, `"total":1`, `"last":`},
		},
		{
			name:   "list defaults to newest first",
			method: http.MethodGet,
			path:   "/api/admin/users",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), ListRequest{Sort: "-created_at"}).Return([]UserResource{}, pagination.NewMeta(1, 20, 0), nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: []string{`"data":[]`},
		},
		{
			name:       "list: an invalid is_active is a 400 on the parameter",
			method:     http.MethodGet,
			path:       "/api/admin/users?filter%5Bis_active%5D=maybe",
			stub:       func(s *MockS) {},
			wantCode:   http.StatusBadRequest,
			wantInBody: []string{`"source":{"parameter":"filter[is_active]"}`},
		},
		{
			name:     "list: include is not supported",
			method:   http.MethodGet,
			path:     "/api/admin/users?include=notes",
			stub:     func(s *MockS) {},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "list: service error",
			method: http.MethodGet,
			path:   "/api/admin/users",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, pagination.Meta{}, errors.New("boom"))
			},
			wantCode: http.StatusInternalServerError,
		},
		{
			name:   "deactivate",
			method: http.MethodPatch,
			path:   "/api/admin/users/3",
			body:   doc("3", `{"is_active":false}`),
			stub: func(s *MockS) {
				s.EXPECT().Update(gomock.Any(), int64(3), UpdateRequest{IsActive: &inactive}, int64(1)).Return(UserResource{ID: "3", IsActive: false}, nil)
			},
			wantCode:   http.StatusOK,
			wantInBody: []string{`"is_active":false`},
		},
		{
			name:   "no attributes: a no-op",
			method: http.MethodPatch,
			path:   "/api/admin/users/3",
			body:   `{"data":{"type":"users","id":"3"}}`,
			stub: func(s *MockS) {
				s.EXPECT().Update(gomock.Any(), int64(3), UpdateRequest{}, int64(1)).Return(UserResource{ID: "3", IsActive: true}, nil)
			},
			wantCode: http.StatusOK,
		},
		{
			name:       "the role can't be set here: 403",
			method:     http.MethodPatch,
			path:       "/api/admin/users/3",
			body:       doc("3", `{"role":"admin"}`),
			stub:       func(s *MockS) {},
			wantCode:   http.StatusForbidden,
			wantInBody: []string{`"pointer":"/data/attributes/role"`},
		},
		{
			name:     "a bad id is a 404",
			method:   http.MethodPatch,
			path:     "/api/admin/users/x",
			body:     doc("x", `{"is_active":true}`),
			stub:     func(s *MockS) {},
			wantCode: http.StatusNotFound,
		},
		{
			name:   "self-deactivation conflict",
			method: http.MethodPatch,
			path:   "/api/admin/users/1",
			body:   doc("1", `{"is_active":false}`),
			stub: func(s *MockS) {
				s.EXPECT().Update(gomock.Any(), int64(1), UpdateRequest{IsActive: &inactive}, int64(1)).
					Return(UserResource{}, apperror.New(apperror.CodeConflict, "You can't deactivate your own account.", nil))
			},
			wantCode: http.StatusConflict,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := NewMockS(ctrl)
			tc.stub(svc)

			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set(internalsharedconstant.UserIdKey, int64(1)); c.Next() })
			Route(r.Group("/api/admin"), NewHandler(svc))

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}
		})
	}
}
