package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	adminuser "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user"
	adminusersqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user/sqlc"
	authlogin "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login"
	authloginsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authlogout "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout"
	authrefresh "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh"
	authregister "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register"
	authregistersqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me"
	mesqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me/sqlc"
	createnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create"
	createnotesqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create/sqlc"
	deletenote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/delete"
	detailnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/detail"
	listnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list"
	listnotesqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list/sqlc"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	updatenote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/update"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

// newIntegrationRouter wires every slice exactly like cmd/api/main.go, but on
// the test container's database and a throwaway ES256 key.
func newIntegrationRouter(t *testing.T, db *sql.DB) *gin.Engine {
	t.Helper()

	require.NoError(t, validation.RegisterGinValidators())

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tokenMaker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "test-client", "test-issuer")

	const accessTTL, refreshTTL = 15 * time.Minute, 24 * time.Hour
	cookies := authshared.CookieConfig{}
	authSharedQuerier := authsharedsqlc.New(db)
	noteSharedQuerier := notesharedsqlc.New(db)

	handlers := Handlers{
		Register: authregister.NewHandler(authregister.NewService(authregistersqlc.New(db))),
		Login: authlogin.NewHandler(
			authlogin.NewService(authloginsqlc.New(db), authSharedQuerier, tokenMaker, accessTTL, refreshTTL),
			cookies,
		),
		Refresh: authrefresh.NewHandler(
			authrefresh.NewService(authrefresh.NewStore(db), tokenMaker, accessTTL, refreshTTL),
			cookies,
		),
		Logout: authlogout.NewHandler(authlogout.NewService(authSharedQuerier, tokenMaker), cookies),

		Me: me.NewHandler(me.NewService(mesqlc.New(db))),

		CreateNote: createnote.NewHandler(createnote.NewService(noteSharedQuerier, createnotesqlc.New(db))),
		ListNote:   listnote.NewHandler(listnote.NewService(listnotesqlc.New(db))),
		DetailNote: detailnote.NewHandler(detailnote.NewService(noteSharedQuerier)),
		UpdateNote: updatenote.NewHandler(updatenote.NewService(updatenote.NewStore(db))),
		DeleteNote: deletenote.NewHandler(deletenote.NewService(deletenote.NewStore(db))),

		AdminUser: adminuser.NewHandler(adminuser.NewService(adminusersqlc.New(db))),
	}

	return NewRouter("test-version", tokenMaker, middlewaresqlc.New(db), handlers)
}

// apiClient sends JSON:API requests to the router in-process.
type apiClient struct {
	t      *testing.T
	router *gin.Engine
}

type apiResponse struct {
	Code   int
	Header http.Header
	Body   map[string]any
}

// do sends one request; body (when set) goes as application/vnd.api+json and
// token (when set) as a bearer access token.
func (c apiClient) do(method, path, body, token string) apiResponse {
	c.t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Accept", "application/vnd.api+json")
	if body != "" {
		req.Header.Set("Content-Type", "application/vnd.api+json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)

	res := apiResponse{Code: rec.Code, Header: rec.Header()}
	if rec.Body.Len() > 0 {
		require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &res.Body), "body: %s", rec.Body.String())
	}
	return res
}

// login signs in with the seeded default password and returns the access token.
func (c apiClient) login(email string) string {
	c.t.Helper()

	res := c.do(http.MethodPost, "/api/auth/login",
		`{"data":{"type":"sessions","attributes":{"email":"`+email+`","password":"`+testhelper.DefaultPassword+`"}}}`, "")
	require.Equal(c.t, http.StatusOK, res.Code, "login: %v", res.Body)
	return res.Body["meta"].(map[string]any)["access_token"].(string)
}

// dataAttr reads data.attributes[name] of a single-resource document.
func dataAttr(t *testing.T, res apiResponse, name string) any {
	t.Helper()
	return res.Body["data"].(map[string]any)["attributes"].(map[string]any)[name]
}

// TestRouter_EndToEnd_I drives the real router, middleware, handlers,
// services and sqlc against a migrated Postgres container: every layer at
// once, over HTTP.
func TestRouter_EndToEnd_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	router := newIntegrationRouter(t, testDB)

	testCases := []struct {
		name string
		run  func(t *testing.T, c apiClient)
	}{
		{
			name: "register, login, then the note lifecycle: create → list → get → update → delete",
			run: func(t *testing.T, c apiClient) {
				res := c.do(http.MethodPost, "/api/auth/register",
					`{"data":{"type":"users","attributes":{"name":"Jane","email":"jane@example.com","password":"`+testhelper.DefaultPassword+`"}}}`, "")
				require.Equal(t, http.StatusCreated, res.Code, "register: %v", res.Body)
				require.Equal(t, "user", dataAttr(t, res, "role"))

				token := c.login("jane@example.com")

				res = c.do(http.MethodGet, "/api/me", "", token)
				require.Equal(t, http.StatusOK, res.Code)
				require.Equal(t, "jane@example.com", dataAttr(t, res, "email"))

				res = c.do(http.MethodPost, "/api/notes", `{"data":{"type":"notes","attributes":{"title":"First","body":"hello"}}}`, token)
				require.Equal(t, http.StatusCreated, res.Code, "create: %v", res.Body)
				id := res.Body["data"].(map[string]any)["id"].(string)
				require.Equal(t, "/api/notes/"+id, res.Header.Get("Location"))

				res = c.do(http.MethodGet, "/api/notes", "", token)
				require.Equal(t, http.StatusOK, res.Code)
				require.Len(t, res.Body["data"], 1)

				res = c.do(http.MethodGet, "/api/notes/"+id, "", token)
				require.Equal(t, http.StatusOK, res.Code)
				require.Equal(t, "First", dataAttr(t, res, "title"))
				require.Len(t, res.Body["included"], 1, "the creator is included by default")

				res = c.do(http.MethodPatch, "/api/notes/"+id, `{"data":{"type":"notes","id":"`+id+`","attributes":{"title":"Renamed"}}}`, token)
				require.Equal(t, http.StatusOK, res.Code, "update: %v", res.Body)
				require.Equal(t, "Renamed", dataAttr(t, res, "title"))
				require.Equal(t, "hello", dataAttr(t, res, "body"), "a partial update keeps the body")

				res = c.do(http.MethodDelete, "/api/notes/"+id, "", token)
				require.Equal(t, http.StatusNoContent, res.Code)

				res = c.do(http.MethodGet, "/api/notes/"+id, "", token)
				require.Equal(t, http.StatusNotFound, res.Code)
			},
		},
		{
			name: "another user's note reads as 404 and can't be changed",
			run: func(t *testing.T, c apiClient) {
				owner := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
				noteID := testhelper.SeedNote(t, testDB, testhelper.SeedNoteParams{Title: "Private", CreatedBy: owner})
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "intruder@example.com"})
				token := c.login("intruder@example.com")

				path := "/api/notes/" + strconv.FormatInt(noteID, 10)
				require.Equal(t, http.StatusNotFound, c.do(http.MethodGet, path, "", token).Code)
				require.Equal(t, http.StatusNotFound, c.do(http.MethodDelete, path, "", token).Code)
				require.False(t, testhelper.GetNote(t, testDB, noteID).DeletedAt.Valid)
			},
		},
		{
			name: "a user deactivated after login is rejected on the next request",
			run: func(t *testing.T, c apiClient) {
				id := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "gone@example.com"})
				token := c.login("gone@example.com")
				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", token).Code)

				_, err := testDB.Exec(`UPDATE users SET is_active = false WHERE id = $1`, id)
				require.NoError(t, err)

				require.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/api/me", "", token).Code)
			},
		},
		{
			name: "the admin group: a plain user is a 403, an admin gets the directory",
			run: func(t *testing.T, c apiClient) {
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "user@example.com"})
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "admin@example.com", Role: "admin"})

				res := c.do(http.MethodGet, "/api/admin/users", "", c.login("user@example.com"))
				require.Equal(t, http.StatusForbidden, res.Code)

				res = c.do(http.MethodGet, "/api/admin/users", "", c.login("admin@example.com"))
				require.Equal(t, http.StatusOK, res.Code)
				require.Len(t, res.Body["data"], 2)
			},
		},
		{
			name: "no token is a 401",
			run: func(t *testing.T, c apiClient) {
				res := c.do(http.MethodGet, "/api/notes", "", "")
				require.Equal(t, http.StatusUnauthorized, res.Code)
				require.NotEmpty(t, res.Body["errors"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			tc.run(t, apiClient{t: t, router: router})
		})
	}
}
