package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
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

			r, err := NewRouter("test-version", maker, users, h.handlers(), Security{
				AuthIPLimit:   ratelimit.NewMemory(ratelimit.Rate{Burst: 100, Per: time.Minute}),
				PublicIPLimit: ratelimit.NewMemory(ratelimit.Rate{Burst: 100, Per: time.Minute}),
			})
			require.NoError(t, err)

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

// TestNewRouter_Security_U pins the base protection: register and login share
// one per-IP auth budget and also spend from the general per-IP budget,
// refresh and logout have none, authenticated routes aren't IP-limited, and
// the client IP comes from X-Forwarded-For only when the connection is a
// trusted proxy.
func TestNewRouter_Security_U(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type call struct {
		method, path string
		remote       string // the connection's IP
		xff          string // X-Forwarded-For, "" = none
		wantCode     int
	}
	ok := func(c *gin.Context) { c.Status(http.StatusTeapot) }
	login := func(remote, xff string, want int) call {
		return call{method: http.MethodPost, path: "/api/auth/login", remote: remote, xff: xff, wantCode: want}
	}

	testCases := []struct {
		name    string
		trusted []string
		// publicBurst is the general per-IP limit (0 = 100, out of the way);
		// the auth limit is 2 per window unless authBurst is set.
		publicBurst, authBurst int
		calls                  []call
	}{
		{name: "login past the per-IP limit is a 429", calls: []call{
			login("10.0.0.1", "", http.StatusTeapot), login("10.0.0.1", "", http.StatusTeapot),
			login("10.0.0.1", "", http.StatusTooManyRequests),
		}},
		{name: "register past the per-IP limit is a 429", calls: []call{
			{method: http.MethodPost, path: "/api/auth/register", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/register", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/register", remote: "10.0.0.1", wantCode: http.StatusTooManyRequests},
		}},
		{name: "register and login share one per-IP budget", calls: []call{
			{method: http.MethodPost, path: "/api/auth/register", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/register", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			login("10.0.0.1", "", http.StatusTooManyRequests),
		}},
		{name: "register and login also spend from the general per-IP budget", publicBurst: 2, authBurst: 100, calls: []call{
			{method: http.MethodPost, path: "/api/auth/register", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			login("10.0.0.1", "", http.StatusTeapot),
			login("10.0.0.1", "", http.StatusTooManyRequests),
		}},
		{name: "the general budget is per IP", publicBurst: 1, authBurst: 100, calls: []call{
			login("10.0.0.1", "", http.StatusTeapot), login("10.0.0.1", "", http.StatusTooManyRequests),
			login("10.0.0.2", "", http.StatusTeapot),
		}},
		{name: "refresh is not limited by the general per-IP budget either", publicBurst: 1, calls: []call{
			{method: http.MethodPost, path: "/api/auth/refresh", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/refresh", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/refresh", remote: "10.0.0.1", wantCode: http.StatusTeapot},
		}},
		{name: "logout is not limited by the general per-IP budget either", publicBurst: 1, calls: []call{
			{method: http.MethodPost, path: "/api/auth/logout", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/logout", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/logout", remote: "10.0.0.1", wantCode: http.StatusTeapot},
		}},
		{name: "an authenticated route is not IP-limited (401 without a token, never 429)", publicBurst: 1, authBurst: 1, calls: []call{
			{method: http.MethodGet, path: "/api/me", remote: "10.0.0.1", wantCode: http.StatusUnauthorized},
			{method: http.MethodGet, path: "/api/me", remote: "10.0.0.1", wantCode: http.StatusUnauthorized},
			{method: http.MethodGet, path: "/api/me", remote: "10.0.0.1", wantCode: http.StatusUnauthorized},
		}},
		{name: "refresh is never IP-limited (every open tab calls it)", calls: []call{
			{method: http.MethodPost, path: "/api/auth/refresh", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/refresh", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/refresh", remote: "10.0.0.1", wantCode: http.StatusTeapot},
		}},
		{name: "logout is never IP-limited", calls: []call{
			{method: http.MethodPost, path: "/api/auth/logout", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/logout", remote: "10.0.0.1", wantCode: http.StatusTeapot},
			{method: http.MethodPost, path: "/api/auth/logout", remote: "10.0.0.1", wantCode: http.StatusTeapot},
		}},
		{name: "with no trusted proxy a forged X-Forwarded-For doesn't escape the limit", calls: []call{
			login("10.0.0.1", "1.1.1.1", http.StatusTeapot), login("10.0.0.1", "2.2.2.2", http.StatusTeapot),
			login("10.0.0.1", "3.3.3.3", http.StatusTooManyRequests),
		}},
		{name: "an untrusted peer's X-Forwarded-For is ignored even when proxies are configured", trusted: []string{"10.0.0.9"}, calls: []call{
			login("10.0.0.1", "1.1.1.1", http.StatusTeapot), login("10.0.0.1", "2.2.2.2", http.StatusTeapot),
			login("10.0.0.1", "3.3.3.3", http.StatusTooManyRequests),
		}},
		{name: "behind a trusted proxy each forwarded client has its own budget", trusted: []string{"10.0.0.0/24"}, calls: []call{
			login("10.0.0.9", "1.1.1.1", http.StatusTeapot), login("10.0.0.9", "1.1.1.1", http.StatusTeapot),
			login("10.0.0.9", "2.2.2.2", http.StatusTeapot),
			login("10.0.0.9", "1.1.1.1", http.StatusTooManyRequests),
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			h := newMocks(ctrl)
			h.register.EXPECT().Register(gomock.Any()).Do(ok).AnyTimes()
			h.login.EXPECT().Login(gomock.Any()).Do(ok).AnyTimes()
			h.refresh.EXPECT().Refresh(gomock.Any()).Do(ok).AnyTimes()
			h.logout.EXPECT().Logout(gomock.Any()).Do(ok).AnyTimes()

			publicBurst, authBurst := tc.publicBurst, tc.authBurst
			if publicBurst == 0 {
				publicBurst = 100
			}
			if authBurst == 0 {
				authBurst = 2
			}
			r, err := NewRouter("test-version", jwttoken.NewMockM(ctrl), middlewaresqlc.NewMockQuerier(ctrl), h.handlers(),
				Security{
					TrustedProxies: tc.trusted,
					AuthIPLimit:    ratelimit.NewMemory(ratelimit.Rate{Burst: authBurst, Per: time.Minute}),
					PublicIPLimit:  ratelimit.NewMemory(ratelimit.Rate{Burst: publicBurst, Per: time.Minute}),
				})
			require.NoError(t, err)

			for i, c := range tc.calls {
				req := httptest.NewRequest(c.method, c.path, nil)
				req.RemoteAddr = c.remote + ":4321"
				if c.xff != "" {
					req.Header.Set("X-Forwarded-For", c.xff)
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.Equal(t, c.wantCode, w.Code, "call %d: %s", i, w.Body.String())
			}
		})
	}
}

// TestNewRouter_CORS_U pins where CORS sits: on the engine, so a browser
// preflight on an /api route (which declares no OPTIONS) is answered by CORS,
// not by the 405 handler.
func TestNewRouter_CORS_U(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name       string
		origin     string
		wantCode   int
		wantOrigin string
	}{
		{name: "a preflight on login from a listed origin reaches CORS (204, not 405)",
			origin: "https://app.example.com", wantCode: http.StatusNoContent, wantOrigin: "https://app.example.com"},
		{name: "a preflight on login from an unlisted origin is CORS's 403, not 405",
			origin: "https://evil.example", wantCode: http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			r, err := NewRouter("v", jwttoken.NewMockM(ctrl), middlewaresqlc.NewMockQuerier(ctrl), newMocks(ctrl).handlers(),
				Security{
					AuthIPLimit:    ratelimit.NewMemory(ratelimit.Rate{Burst: 10, Per: time.Minute}),
					PublicIPLimit:  ratelimit.NewMemory(ratelimit.Rate{Burst: 10, Per: time.Minute}),
					AllowedOrigins: []string{"https://app.example.com"},
				})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Equal(t, tc.wantOrigin, w.Header().Get("Access-Control-Allow-Origin"))
			require.NotEmpty(t, w.Header().Get("X-Request-ID"), "CORS runs after RequestID")
		})
	}
}

func TestNewRouter_SecurityErrors_U(t *testing.T) {
	testCases := []struct {
		name    string
		sec     Security
		wantErr string
	}{
		{name: "no auth IP limit: refused, the auth routes must never run unlimited",
			sec: Security{}, wantErr: "AuthIPLimit is required"},
		{name: "a trusted proxy gin can't parse is refused",
			sec: Security{TrustedProxies: []string{"not-an-ip"}, AuthIPLimit: ratelimit.NewMemory(ratelimit.Rate{Burst: 1, Per: time.Minute}),
				PublicIPLimit: ratelimit.NewMemory(ratelimit.Rate{Burst: 1, Per: time.Minute})}, wantErr: "trusted proxies"},
		{name: "no general IP limit: refused, the public routes must never run unlimited",
			sec: Security{AuthIPLimit: ratelimit.NewMemory(ratelimit.Rate{Burst: 1, Per: time.Minute})}, wantErr: "PublicIPLimit is required"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			r, err := NewRouter("v", jwttoken.NewMockM(ctrl), middlewaresqlc.NewMockQuerier(ctrl), newMocks(ctrl).handlers(), tc.sec)
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, r)
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
