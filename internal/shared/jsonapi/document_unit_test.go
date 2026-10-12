package jsonapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
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

func TestWriteErrorsContext_Log_U(t *testing.T) {
	live := func() context.Context { return requestid.With(context.Background(), "ctx-id") }
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(requestid.With(context.Background(), "ctx-id"))
		cancel()
		return ctx
	}
	expired := func() context.Context {
		ctx, cancel := context.WithDeadline(requestid.With(context.Background(), "ctx-id"), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		return ctx
	}

	testCases := []struct {
		name      string
		ctx       func() context.Context
		headerID  string // set on the response header, as middleware.RequestID does
		err       *apperror.AppError
		wantLog   bool
		wantLevel string
		wantMsg   string
		wantID    string // "" => no request_id attribute
	}{
		{
			name: "a live 500: request failed at error", ctx: live, headerID: "ctx-id",
			err:     apperror.New(apperror.CodeInternal, "Oops.", errors.New("db down")),
			wantLog: true, wantLevel: "ERROR", wantMsg: "request failed", wantID: "ctx-id",
		},
		{
			name: "a cancelled ctx with a pq-style error: request cancelled at warn", ctx: cancelled, headerID: "ctx-id",
			err:     apperror.New(apperror.CodeInternal, "Oops.", errors.New("pq: canceling statement due to user request")),
			wantLog: true, wantLevel: "WARN", wantMsg: "request cancelled", wantID: "ctx-id",
		},
		{
			name: "context.Canceled in the chain: request cancelled at warn", ctx: live,
			err:     apperror.New(apperror.CodeInternal, "Oops.", fmt.Errorf("get note: %w", context.Canceled)),
			wantLog: true, wantLevel: "WARN", wantMsg: "request cancelled", wantID: "ctx-id",
		},
		{
			name: "a deadline exceeded is our timeout: request failed at error", ctx: expired,
			err:     apperror.New(apperror.CodeInternal, "Oops.", fmt.Errorf("get note: %w", context.DeadlineExceeded)),
			wantLog: true, wantLevel: "ERROR", wantMsg: "request failed", wantID: "ctx-id",
		},
		{
			name: "a cancelled 404 is still request rejected", ctx: cancelled,
			err:     apperror.New(apperror.CodeNotFound, "Gone.", errors.New("no rows")),
			wantLog: true, wantLevel: "WARN", wantMsg: "request rejected", wantID: "ctx-id",
		},
		{
			name: "no id in ctx: read from the response header", ctx: context.Background, headerID: "header-id",
			err:     apperror.New(apperror.CodeInternal, "Oops.", errors.New("db down")),
			wantLog: true, wantLevel: "ERROR", wantMsg: "request failed", wantID: "header-id",
		},
		{
			name: "no id anywhere", ctx: context.Background,
			err:     apperror.New(apperror.CodeConflict, "Wrong type.", errors.New("type mismatch")),
			wantLog: true, wantLevel: "WARN", wantMsg: "request rejected",
		},
		{
			name: "no wrapped cause: nothing logged", ctx: cancelled,
			err: apperror.New(apperror.CodeNotFound, "Gone.", nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			w := httptest.NewRecorder()
			if tc.headerID != "" {
				w.Header().Set(requestid.Header, tc.headerID)
			}
			WriteErrorsContext(tc.ctx(), w, tc.err)

			require.Equal(t, apperror.MapToHTTPStatus(tc.err.Code), w.Code)
			if tc.err.Err != nil {
				require.NotContains(t, w.Body.String(), tc.err.Err.Error(), "the cause is never in the body")
			}
			if !tc.wantLog {
				require.Empty(t, buf.String())
				return
			}

			line := bytes.TrimSpace(buf.Bytes())
			var rec map[string]any
			require.NoError(t, json.Unmarshal(line, &rec), buf.String())
			require.Equal(t, tc.wantLevel, rec["level"])
			require.Equal(t, tc.wantMsg, rec["msg"])
			require.Equal(t, float64(w.Code), rec["status"])
			require.Equal(t, string(tc.err.Code), rec["code"])
			require.Equal(t, tc.err.Err.Error(), rec["err"])
			if tc.wantID != "" {
				require.Equal(t, tc.wantID, rec[requestid.Key])
				require.Equal(t, 1, strings.Count(string(line), `"`+requestid.Key+`"`), "request_id logged exactly once")
			} else {
				require.NotContains(t, rec, requestid.Key)
			}
		})
	}
}

func TestWriteErrors_UsesBackground_U(t *testing.T) {
	testCases := []struct {
		name      string
		err       *apperror.AppError
		wantLevel string
		wantMsg   string
	}{
		{name: "a 500 is request failed", err: apperror.New(apperror.CodeInternal, "Oops.", errors.New("db down")), wantLevel: "ERROR", wantMsg: "request failed"},
		{name: "a 409 is request rejected", err: apperror.New(apperror.CodeConflict, "No.", errors.New("mismatch")), wantLevel: "WARN", wantMsg: "request rejected"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			w := httptest.NewRecorder()
			w.Header().Set(requestid.Header, "req-1")
			WriteErrors(w, tc.err)

			var rec map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec), buf.String())
			require.Equal(t, tc.wantLevel, rec["level"])
			require.Equal(t, tc.wantMsg, rec["msg"])
			require.Equal(t, "req-1", rec[requestid.Key])
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
