package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	adminuser "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user"
	authlogin "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login"
	authlogout "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout"
	authrefresh "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh"
	authregister "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me"
	createnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create"
	deletenote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/delete"
	detailnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/detail"
	listnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list"
	updatenote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/update"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
	"github.com/stretchr/testify/require"
)

// TestNewRouter_U checks each route reaches its handler behind the right
// middleware: public routes without a token, authed routes only with one,
// admin routes only for an admin.
func TestNewRouter_U(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ok := func(c *gin.Context) { c.Status(http.StatusTeapot) }

	testCases := []struct {
		name     string
		method   string
		path     string
		body     string // sent as application/json when set
		role     string // "" => no token
		expect   func(h *mocks)
		wantCode int
		wantBody string // exact body when set
	}{
		{name: "healthz is public and reports status + version", method: http.MethodGet, path: "/healthz", wantCode: http.StatusOK,
			wantBody: `{"jsonapi":{"version":"1.1"},"meta":{"status":"up","version":"test-version"},"links":{"self":"/healthz"}}`},
		{name: "an unknown route is a JSON:API 404", method: http.MethodGet, path: "/api/nope", wantCode: http.StatusNotFound},
		{name: "a wrong method is a JSON:API 405", method: http.MethodPut, path: "/api/auth/login", wantCode: http.StatusMethodNotAllowed},
		{name: "a panic is a JSON:API 500", method: http.MethodGet, path: "/api/me", role: "user",
			expect:   func(h *mocks) { h.me.EXPECT().Get(gomock.Any()).Do(func(*gin.Context) { panic("boom") }) },
			wantCode: http.StatusInternalServerError},
		{name: "a non JSON:API body is a 415 before any handler", method: http.MethodPost, path: "/api/auth/login",
			body: `{}`, wantCode: http.StatusUnsupportedMediaType},
		{name: "register is public", method: http.MethodPost, path: "/api/auth/register",
			expect: func(h *mocks) { h.register.EXPECT().Register(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "login is public", method: http.MethodPost, path: "/api/auth/login",
			expect: func(h *mocks) { h.login.EXPECT().Login(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "refresh is public", method: http.MethodPost, path: "/api/auth/refresh",
			expect: func(h *mocks) { h.refresh.EXPECT().Refresh(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "logout is public", method: http.MethodPost, path: "/api/auth/logout",
			expect: func(h *mocks) { h.logout.EXPECT().Logout(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "me needs a token", method: http.MethodGet, path: "/api/me", wantCode: http.StatusUnauthorized},
		{name: "me with a token", method: http.MethodGet, path: "/api/me", role: "user",
			expect: func(h *mocks) { h.me.EXPECT().Get(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "create note", method: http.MethodPost, path: "/api/notes", role: "user",
			expect: func(h *mocks) { h.create.EXPECT().Create(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "list notes", method: http.MethodGet, path: "/api/notes", role: "user",
			expect: func(h *mocks) { h.list.EXPECT().List(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "note detail", method: http.MethodGet, path: "/api/notes/1", role: "user",
			expect: func(h *mocks) { h.detail.EXPECT().Get(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "update note", method: http.MethodPatch, path: "/api/notes/1", role: "user",
			expect: func(h *mocks) { h.update.EXPECT().Update(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "delete note", method: http.MethodDelete, path: "/api/notes/1", role: "user",
			expect: func(h *mocks) { h.delete.EXPECT().Delete(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "admin route refuses a user", method: http.MethodGet, path: "/api/admin/users", role: "user", wantCode: http.StatusForbidden},
		{name: "admin route for an admin", method: http.MethodGet, path: "/api/admin/users", role: "admin",
			expect: func(h *mocks) { h.adminUser.EXPECT().List(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
		{name: "admin update user", method: http.MethodPatch, path: "/api/admin/users/2", role: "admin",
			expect: func(h *mocks) { h.adminUser.EXPECT().Update(gomock.Any()).Do(ok) }, wantCode: http.StatusTeapot},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			h := newMocks(ctrl)
			if tc.expect != nil {
				tc.expect(h)
			}

			maker := jwttoken.NewMockM(ctrl)
			users := middlewaresqlc.NewMockQuerier(ctrl)
			if tc.role != "" {
				maker.EXPECT().VerifyAccessToken("token").Return(jwt.MapClaims{"sub": float64(1), "role": tc.role}, nil)
				users.EXPECT().GetUserIsActive(gomock.Any(), int64(1)).Return(true, nil)
			}

			r := NewRouter("test-version", maker, users, h.handlers())

			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.role != "" {
				req.Header.Set("Authorization", "Bearer token")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.NotEmpty(t, w.Header().Get("X-Request-ID"), "every response carries a request id")
			if tc.wantCode != http.StatusTeapot {
				require.Equal(t, jsonapi.MediaType, w.Header().Get("Content-Type"))
			}
			if tc.wantBody != "" {
				require.JSONEq(t, tc.wantBody, w.Body.String())
			}
		})
	}
}

type mocks struct {
	register  *authregister.MockH
	login     *authlogin.MockH
	refresh   *authrefresh.MockH
	logout    *authlogout.MockH
	me        *me.MockH
	create    *createnote.MockH
	list      *listnote.MockH
	detail    *detailnote.MockH
	update    *updatenote.MockH
	delete    *deletenote.MockH
	adminUser *adminuser.MockH
}

func newMocks(ctrl *gomock.Controller) *mocks {
	return &mocks{
		register:  authregister.NewMockH(ctrl),
		login:     authlogin.NewMockH(ctrl),
		refresh:   authrefresh.NewMockH(ctrl),
		logout:    authlogout.NewMockH(ctrl),
		me:        me.NewMockH(ctrl),
		create:    createnote.NewMockH(ctrl),
		list:      listnote.NewMockH(ctrl),
		detail:    detailnote.NewMockH(ctrl),
		update:    updatenote.NewMockH(ctrl),
		delete:    deletenote.NewMockH(ctrl),
		adminUser: adminuser.NewMockH(ctrl),
	}
}

func (m *mocks) handlers() Handlers {
	return Handlers{
		Register:   m.register,
		Login:      m.login,
		Refresh:    m.refresh,
		Logout:     m.logout,
		Me:         m.me,
		CreateNote: m.create,
		ListNote:   m.list,
		DetailNote: m.detail,
		UpdateNote: m.update,
		DeleteNote: m.delete,
		AdminUser:  m.adminUser,
	}
}
