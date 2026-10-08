package jsonapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

func TestWriteData_U(t *testing.T) {
	user := testUser{ID: 1, Name: "Jane"}

	testCases := []struct {
		name   string
		target string
		status int
		data   any
		q      Query
		meta   any
		want   string
	}{
		{
			name:   "a single resource: jsonapi version + self link",
			target: "/api/users/1?fields%5Busers%5D=name",
			status: http.StatusOK,
			data:   user,
			q:      Query{Fields: Fieldsets{"users": {"name": true}}},
			want: `{"jsonapi":{"version":"1.1"},"data":{"type":"users","id":"1","attributes":{"name":"Jane"}},` +
				`"links":{"self":"/api/users/1?fields%5Busers%5D=name"}}`,
		},
		{
			name:   "a page of a collection: meta + pagination links",
			target: "/api/users?page%5Bnumber%5D=2&page%5Bsize%5D=1",
			status: http.StatusOK,
			data:   []testUser{user},
			q:      Query{Fields: Fieldsets{"users": {"name": true}}},
			meta:   pagination.NewMeta(2, 1, 3),
			want: `{"jsonapi":{"version":"1.1"},"data":[{"type":"users","id":"1","attributes":{"name":"Jane"}}],` +
				`"meta":{"page":2,"page_size":1,"total":3,"total_page":3},` +
				`"links":{"first":"/api/users?page%5Bnumber%5D=1&page%5Bsize%5D=1",` +
				`"last":"/api/users?page%5Bnumber%5D=3&page%5Bsize%5D=1",` +
				`"next":"/api/users?page%5Bnumber%5D=3&page%5Bsize%5D=1",` +
				`"prev":"/api/users?page%5Bnumber%5D=1&page%5Bsize%5D=1",` +
				`"self":"/api/users?page%5Bnumber%5D=2&page%5Bsize%5D=1"}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteData(w, httptest.NewRequest(http.MethodGet, tc.target, nil), tc.status, "users", tc.data, tc.q, tc.meta)

			require.Equal(t, tc.status, w.Code)
			require.Equal(t, MediaType, w.Header().Get("Content-Type"))
			require.JSONEq(t, tc.want, w.Body.String())
		})
	}
}

func TestWriteMeta_U(t *testing.T) {
	testCases := []struct {
		name string
		meta any
		want string
	}{
		{
			name: "a meta-only document",
			meta: map[string]string{"status": "ok"},
			want: `{"jsonapi":{"version":"1.1"},"meta":{"status":"ok"},"links":{"self":"/x"}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteMeta(w, httptest.NewRequest(http.MethodPost, "/x", nil), http.StatusOK, tc.meta)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, MediaType, w.Header().Get("Content-Type"))
			require.JSONEq(t, tc.want, w.Body.String())
		})
	}
}

func TestWriteErrors_U(t *testing.T) {
	testCases := []struct {
		name       string
		errs       []*apperror.AppError
		wantStatus int
		want       string
	}{
		{
			name:       "one error with a source",
			errs:       []*apperror.AppError{apperror.New(apperror.CodeConflict, "Wrong type.", errors.New("log me")).WithPointer("/data/type")},
			wantStatus: http.StatusConflict,
			want: `{"jsonapi":{"version":"1.1"},"errors":[{"status":"409","code":"conflict","title":"Conflict",` +
				`"detail":"Wrong type.","source":{"pointer":"/data/type"}}]}`,
		},
		{
			name: "several of one status keep it",
			errs: []*apperror.AppError{
				apperror.New(apperror.CodeUnprocessableEntity, "A.", nil),
				apperror.New(apperror.CodeUnprocessableEntity, "B.", nil),
			},
			wantStatus: http.StatusUnprocessableEntity,
			want: `{"jsonapi":{"version":"1.1"},"errors":[` +
				`{"status":"422","code":"unprocessable_entity","title":"Unprocessable Entity","detail":"A."},` +
				`{"status":"422","code":"unprocessable_entity","title":"Unprocessable Entity","detail":"B."}]}`,
		},
		{
			name: "mixed 4xx: 400",
			errs: []*apperror.AppError{
				apperror.New(apperror.CodeForbidden, "A.", nil),
				apperror.New(apperror.CodeConflict, "B.", nil),
			},
			wantStatus: http.StatusBadRequest,
			want: `{"jsonapi":{"version":"1.1"},"errors":[` +
				`{"status":"403","code":"forbidden","title":"Forbidden","detail":"A."},` +
				`{"status":"409","code":"conflict","title":"Conflict","detail":"B."}]}`,
		},
		{
			name: "a 5xx among them: 500",
			errs: []*apperror.AppError{
				apperror.New(apperror.CodeConflict, "A.", nil),
				apperror.New(apperror.CodeInternal, "B.", errors.New("db down")),
			},
			wantStatus: http.StatusInternalServerError,
			want: `{"jsonapi":{"version":"1.1"},"errors":[` +
				`{"status":"409","code":"conflict","title":"Conflict","detail":"A."},` +
				`{"status":"500","code":"internal_error","title":"Internal Server Error","detail":"B."}]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteErrors(w, tc.errs...)

			require.Equal(t, tc.wantStatus, w.Code)
			require.Equal(t, MediaType, w.Header().Get("Content-Type"))
			require.JSONEq(t, tc.want, w.Body.String())
		})
	}
}

func TestWriteError_Helpers_U(t *testing.T) {
	testCases := []struct {
		name       string
		write      func(w http.ResponseWriter)
		wantStatus int
		wantErrors []ErrorObject
	}{
		{
			name:       "WriteError",
			write:      func(w http.ResponseWriter) { WriteError(w, apperror.CodeNotFound, "Gone.", nil) },
			wantStatus: http.StatusNotFound,
			wantErrors: []ErrorObject{{Status: "404", Code: "not_found", Title: "Not Found", Detail: "Gone."}},
		},
		{
			name: "WriteAppError",
			write: func(w http.ResponseWriter) {
				WriteAppError(w, apperror.New(apperror.CodeBadRequest, "Bad.", nil).WithParameter("sort"))
			},
			wantStatus: http.StatusBadRequest,
			wantErrors: []ErrorObject{{Status: "400", Code: "bad_request", Title: "Bad Request", Detail: "Bad.",
				Source: &apperror.Source{Parameter: "sort"}}},
		},
		{
			name: "WriteValidationErrors prefixes each pointer",
			write: func(w http.ResponseWriter) {
				WriteValidationErrors(w, "/data/attributes", apperror.ValidationErrors{
					{Detail: "Title is required.", Pointer: "/title"},
					{Detail: "Body is too long.", Pointer: "/body"},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErrors: []ErrorObject{
				{Status: "422", Code: "unprocessable_entity", Title: "Unprocessable Entity", Detail: "Title is required.",
					Source: &apperror.Source{Pointer: "/data/attributes/title"}},
				{Status: "422", Code: "unprocessable_entity", Title: "Unprocessable Entity", Detail: "Body is too long.",
					Source: &apperror.Source{Pointer: "/data/attributes/body"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.write(w)

			require.Equal(t, tc.wantStatus, w.Code)
			var doc ErrorDocument
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
			require.Equal(t, tc.wantErrors, doc.Errors)
			require.Equal(t, &Object{Version: Version}, doc.JSONAPI)
		})
	}
}

func TestPaginationLinks_U(t *testing.T) {
	u, err := url.Parse("/api/notes?filter%5Bq%5D=hi&page%5Bnumber%5D=2")
	require.NoError(t, err)
	link := func(n int) string {
		return "/api/notes?filter%5Bq%5D=hi&page%5Bnumber%5D=" + string(rune('0'+n)) + "&page%5Bsize%5D=10"
	}

	testCases := []struct {
		name string
		meta pagination.Meta
		want Links
	}{
		{
			name: "a middle page: every link, other parameters kept",
			meta: pagination.NewMeta(2, 10, 25),
			want: Links{"first": link(1), "last": link(3), "prev": link(1), "next": link(3)},
		},
		{
			name: "the first page: no prev",
			meta: pagination.NewMeta(1, 10, 25),
			want: Links{"first": link(1), "last": link(3), "prev": nil, "next": link(2)},
		},
		{
			name: "the last page: no next",
			meta: pagination.NewMeta(3, 10, 25),
			want: Links{"first": link(1), "last": link(3), "prev": link(2), "next": nil},
		},
		{
			name: "no rows: one empty page",
			meta: pagination.NewMeta(1, 10, 0),
			want: Links{"first": link(1), "last": link(1), "prev": nil, "next": nil},
		},
		{
			name: "past the end: prev points at the last page",
			meta: pagination.NewMeta(9, 10, 25),
			want: Links{"first": link(1), "last": link(3), "prev": link(3), "next": nil},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PaginationLinks(u, tc.meta))
		})
	}
}
