package server

import (
	"context"
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
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/activitylog"
	activitylogsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/activitylog/sqlc"
	adminuser "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user"
	adminusersqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user/sqlc"
	authemailverification "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverification"
	authemailverificationrequest "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverificationrequest"
	authemailverificationrequestsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverificationrequest/sqlc"
	authlogin "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login"
	authloginsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authlogout "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout"
	authlogoutsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout/sqlc"
	authpasswordchange "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordchange"
	authpasswordchangesqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordchange/sqlc"
	authpasswordreset "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordreset"
	authpasswordresetrequest "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordresetrequest"
	authpasswordresetrequestsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordresetrequest/sqlc"
	authrefresh "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh"
	authregister "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register"
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
	sessionlist "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/list"
	sessionlistsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/list/sqlc"
	sessionrevoke "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/revoke"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

// newIntegrationRouter wires every slice exactly like cmd/api/main.go, but on
// the test container's database, a throwaway ES256 key and a fake mailer that
// sends inline (the returned one, for the email flows).
func newIntegrationRouter(t *testing.T, db *sql.DB) (*gin.Engine, *mailer.Fake, *activitylog.Recorder) {
	t.Helper()

	require.NoError(t, validation.RegisterGinValidators())

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tokenMaker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "test-client", "test-issuer")

	const accessTTL, refreshTTL, untrustedTTL = 15 * time.Minute, 24 * time.Hour, 24 * time.Hour
	// testLimit keeps the rate limits out of the way: these flows log in many
	// times from httptest's one address. The limits themselves are unit-tested.
	const testLimit = 1000
	cookies := authshared.CookieConfig{}
	authSharedQuerier := authsharedsqlc.New(db)
	noteSharedQuerier := notesharedsqlc.New(db)
	mail := &mailer.Fake{}
	activity := activitylog.NewRecorder(activitylogsqlc.New(db))
	emailer := authshared.Emailer{Mailer: mail, AppBaseURL: "https://app.example.com",
		Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }}

	handlers := Handlers{
		Register: authregister.NewHandler(authregister.NewService(authregister.NewStore(db), emailer)),
		Login: authlogin.NewHandler(
			authlogin.NewService(authloginsqlc.New(db), authSharedQuerier, tokenMaker, accessTTL, refreshTTL, untrustedTTL,
				authlogin.Limiters{
					Email: ratelimit.NewMemory(ratelimit.Rate{Burst: testLimit, Per: time.Minute}),
					IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: testLimit, Per: time.Minute}),
				}),
			cookies,
		),
		Refresh: authrefresh.NewHandler(
			authrefresh.NewService(authrefresh.NewStore(db), tokenMaker, accessTTL, refreshTTL),
			cookies,
		),
		PasswordResetRequest: authpasswordresetrequest.NewHandler(authpasswordresetrequest.NewService(
			authpasswordresetrequestsqlc.New(db), authpasswordresetrequest.NewStore(db),
			ratelimit.NewMemory(ratelimit.Rate{Burst: testLimit, Per: time.Minute}), emailer)),
		EmailVerification: authemailverification.NewHandler(authemailverification.NewService(authemailverification.NewStore(db))),
		EmailVerificationRequest: authemailverificationrequest.NewHandler(authemailverificationrequest.NewService(
			authemailverificationrequestsqlc.New(db), authemailverificationrequest.NewStore(db),
			ratelimit.NewMemory(ratelimit.Rate{Burst: testLimit, Per: time.Minute}), emailer)),
		PasswordChange: authpasswordchange.NewHandler(authpasswordchange.NewService(authpasswordchangesqlc.New(db),
			authpasswordchange.NewStore(db), ratelimit.NewMemory(ratelimit.Rate{Burst: testLimit, Per: time.Minute}), emailer)),
		PasswordReset: authpasswordreset.NewHandler(authpasswordreset.NewService(authpasswordreset.NewStore(db), emailer)),
		Logout:        authlogout.NewHandler(authlogout.NewService(authlogoutsqlc.New(db), authSharedQuerier, tokenMaker), cookies),

		Me: me.NewHandler(me.NewService(mesqlc.New(db))),

		ListSessions:  sessionlist.NewHandler(sessionlist.NewService(sessionlistsqlc.New(db))),
		RevokeSession: sessionrevoke.NewHandler(sessionrevoke.NewService(sessionrevoke.NewStore(db)), cookies),

		CreateNote: createnote.NewHandler(createnote.NewService(noteSharedQuerier, createnotesqlc.New(db))),
		ListNote:   listnote.NewHandler(listnote.NewService(listnotesqlc.New(db))),
		DetailNote: detailnote.NewHandler(detailnote.NewService(noteSharedQuerier)),
		UpdateNote: updatenote.NewHandler(updatenote.NewService(updatenote.NewStore(db))),
		DeleteNote: deletenote.NewHandler(deletenote.NewService(deletenote.NewStore(db))),

		AdminUser: adminuser.NewHandler(adminuser.NewService(adminusersqlc.New(db), adminuser.NewStore(db))),
	}

	r, err := NewRouter("test-version", tokenMaker, middlewaresqlc.New(db), handlers,
		Security{
			AuthIPLimit:   ratelimit.NewMemory(ratelimit.Rate{Burst: testLimit, Per: time.Minute}),
			PublicIPLimit: ratelimit.NewMemory(ratelimit.Rate{Burst: testLimit, Per: time.Minute}),

			ActivityRecorder: activity,
		})
	require.NoError(t, err)
	return r, mail, activity
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
	access, _ := c.loginPair(email)
	return access
}

// loginPair signs in and returns the access and the refresh token.
func (c apiClient) loginPair(email string) (access, refresh string) {
	c.t.Helper()

	res := c.do(http.MethodPost, "/api/auth/login",
		`{"data":{"type":"sessions","attributes":{"email":"`+email+`","password":"`+testhelper.DefaultPassword+`"}}}`, "")
	require.Equal(c.t, http.StatusOK, res.Code, "login: %v", res.Body)
	meta := res.Body["meta"].(map[string]any)
	return meta["access_token"].(string), meta["refresh_token"].(string)
}

// errorDetail reads errors[0].detail.
func errorDetail(t *testing.T, res apiResponse) string {
	t.Helper()
	return res.Body["errors"].([]any)[0].(map[string]any)["detail"].(string)
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

	router, mail, activity := newIntegrationRouter(t, testDB)

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
			name: "logout ends the session at once: its still-unexpired access token is a 401, another device's isn't",
			run: func(t *testing.T, c apiClient) {
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
				laptopAccess, laptopRefresh := c.loginPair("jane@example.com")
				phoneAccess := c.login("jane@example.com")
				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", laptopAccess).Code)

				res := c.do(http.MethodPost, "/api/auth/logout",
					`{"data":{"type":"sessions","attributes":{"refresh_token":"`+laptopRefresh+`"}}}`, "")
				require.Equal(t, http.StatusNoContent, res.Code, "logout: %v", res.Body)

				res = c.do(http.MethodGet, "/api/me", "", laptopAccess)
				require.Equal(t, http.StatusUnauthorized, res.Code)
				require.Equal(t, "Your session has ended. Please sign in again.", errorDetail(t, res))
				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", phoneAccess).Code)

				res = c.do(http.MethodPost, "/api/auth/refresh",
					`{"data":{"type":"sessions","attributes":{"refresh_token":"`+laptopRefresh+`"}}}`, "")
				require.Equal(t, http.StatusUnauthorized, res.Code, "the signed-out session can't be refreshed")
				require.Equal(t, "Your session has ended. Please sign in again.", errorDetail(t, res))
			},
		},
		{
			name: "devices: the list marks the current one, signing the other out ends it at once, signing the current out is a logout",
			run: func(t *testing.T, c apiClient) {
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
				laptop := c.login("jane@example.com")
				phoneAccess, phoneRefresh := c.loginPair("jane@example.com")

				res := c.do(http.MethodGet, "/api/me/sessions", "", laptop)
				require.Equal(t, http.StatusOK, res.Code, "list: %v", res.Body)
				sessions := res.Body["data"].([]any)
				require.Len(t, sessions, 2)
				var laptopID, phoneID string
				for _, s := range sessions {
					s := s.(map[string]any)
					if s["attributes"].(map[string]any)["current"].(bool) {
						laptopID = s["id"].(string)
					} else {
						phoneID = s["id"].(string)
					}
				}
				require.NotEmpty(t, laptopID, "the caller's own session is marked current")
				require.NotEmpty(t, phoneID)

				res = c.do(http.MethodDelete, "/api/me/sessions/"+phoneID, "", laptop)
				require.Equal(t, http.StatusNoContent, res.Code, "revoke: %v", res.Body)
				require.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/api/me", "", phoneAccess).Code)
				res = c.do(http.MethodPost, "/api/auth/refresh",
					`{"data":{"type":"sessions","attributes":{"refresh_token":"`+phoneRefresh+`"}}}`, "")
				require.Equal(t, http.StatusUnauthorized, res.Code, "the signed-out device can't refresh")
				require.Equal(t, http.StatusNotFound, c.do(http.MethodDelete, "/api/me/sessions/"+phoneID, "", laptop).Code,
					"a session already signed out reads as missing")

				res = c.do(http.MethodDelete, "/api/me/sessions/"+laptopID, "", laptop)
				require.Equal(t, http.StatusNoContent, res.Code)
				var cleared []string
				for _, line := range res.Header.Values("Set-Cookie") {
					if strings.Contains(line, "Max-Age=0") {
						cleared = append(cleared, strings.SplitN(line, "=", 2)[0])
					}
				}
				require.ElementsMatch(t, []string{"app-access-token", "app-refresh-token"}, cleared)
				require.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/api/me", "", laptop).Code)
			},
		},
		{
			name: "someone else's session reads as missing and stays live",
			run: func(t *testing.T, c apiClient) {
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "bob@example.com"})
				jane := c.login("jane@example.com")
				bob := c.login("bob@example.com")
				bobSession := c.do(http.MethodGet, "/api/me/sessions", "", bob).Body["data"].([]any)[0].(map[string]any)["id"].(string)

				res := c.do(http.MethodDelete, "/api/me/sessions/"+bobSession, "", jane)
				require.Equal(t, http.StatusNotFound, res.Code)
				require.Equal(t, "That session doesn't exist.", errorDetail(t, res))
				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", bob).Code)
				require.Equal(t, http.StatusNotFound, c.do(http.MethodDelete, "/api/me/sessions/not-a-uuid", "", jane).Code)
			},
		},
		{
			name: "an admin deactivating a user ends every session of theirs (refresh: no longer active); reactivating doesn't bring them back (refresh: session ended)",
			run: func(t *testing.T, c apiClient) {
				userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "admin@example.com", Role: "admin"})
				_, janeRefresh := c.loginPair("jane@example.com")
				admin := c.login("admin@example.com")
				path := "/api/admin/users/" + strconv.FormatInt(userID, 10)
				doc := func(active bool) string {
					return `{"data":{"type":"users","id":"` + strconv.FormatInt(userID, 10) + `","attributes":{"is_active":` + strconv.FormatBool(active) + `}}}`
				}

				refresh := func() apiResponse {
					return c.do(http.MethodPost, "/api/auth/refresh",
						`{"data":{"type":"sessions","attributes":{"refresh_token":"`+janeRefresh+`"}}}`, "")
				}

				require.Equal(t, http.StatusOK, c.do(http.MethodPatch, path, doc(false), admin).Code)
				res := refresh()
				require.Equal(t, http.StatusUnauthorized, res.Code, "a deactivated account can't refresh")
				require.Equal(t, "Your account is no longer active.", errorDetail(t, res), "the account is named, not the session")

				require.Equal(t, http.StatusOK, c.do(http.MethodPatch, path, doc(true), admin).Code)
				res = refresh()
				require.Equal(t, http.StatusUnauthorized, res.Code, "the old device stays signed out after reactivation")
				require.Equal(t, "Your session has ended. Please sign in again.", errorDetail(t, res))
			},
		},
		{
			name: "a refresh keeps the session: the new access token works and still names it",
			run: func(t *testing.T, c apiClient) {
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
				_, refresh := c.loginPair("jane@example.com")

				res := c.do(http.MethodPost, "/api/auth/refresh",
					`{"data":{"type":"sessions","attributes":{"refresh_token":"`+refresh+`"}}}`, "")
				require.Equal(t, http.StatusOK, res.Code, "refresh: %v", res.Body)
				access := res.Body["meta"].(map[string]any)["access_token"].(string)
				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", access).Code)
			},
		},
		{
			name: "forgot password: the emailed link sets a new password, signs every device out, and works once",
			run: func(t *testing.T, c apiClient) {
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
				oldAccess := c.login("jane@example.com")
				before := len(mail.Sent()) // the mailer is shared by every case

				res := c.do(http.MethodPost, "/api/auth/password-reset-requests",
					`{"data":{"type":"password-reset-requests","attributes":{"email":"JANE@example.com"}}}`, "")
				require.Equal(t, http.StatusAccepted, res.Code, "request: %v", res.Body)
				sent := mail.Sent()[before:]
				require.Len(t, sent, 1)
				_, afterToken, _ := strings.Cut(sent[0].Text, "token=")
				token, _, _ := strings.Cut(afterToken, "\n")

				reset := `{"data":{"type":"password-resets","attributes":{"token":"` + token + `","password":"N3w-Passw0rd"}}}`
				res = c.do(http.MethodPost, "/api/auth/password-resets", reset, "")
				require.Equal(t, http.StatusNoContent, res.Code, "reset: %v", res.Body)
				require.Equal(t, "Your password was changed", mail.Sent()[before+1].Subject)

				res = c.do(http.MethodGet, "/api/me", "", oldAccess)
				require.Equal(t, http.StatusUnauthorized, res.Code, "every session ended")
				require.Equal(t, "Your session has ended. Please sign in again.", errorDetail(t, res))
				res = c.do(http.MethodPost, "/api/auth/login",
					`{"data":{"type":"sessions","attributes":{"email":"jane@example.com","password":"N3w-Passw0rd"}}}`, "")
				require.Equal(t, http.StatusOK, res.Code, "the new password logs in")

				res = c.do(http.MethodPost, "/api/auth/password-resets", reset, "")
				require.Equal(t, http.StatusUnprocessableEntity, res.Code, "the link works once")
				require.Equal(t, "This link is invalid or has expired. Ask for a new one.", errorDetail(t, res))
			},
		},
		{
			name: "change password: this device stays signed in, the others are signed out at once",
			run: func(t *testing.T, c apiClient) {
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
				laptop := c.login("jane@example.com")
				phone := c.login("jane@example.com")

				res := c.do(http.MethodPost, "/api/auth/password-changes",
					`{"data":{"type":"password-changes","attributes":{"current_password":"`+testhelper.DefaultPassword+`","password":"N3w-Passw0rd"}}}`, laptop)
				require.Equal(t, http.StatusNoContent, res.Code, "change: %v", res.Body)

				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", laptop).Code)
				res = c.do(http.MethodGet, "/api/me", "", phone)
				require.Equal(t, http.StatusUnauthorized, res.Code)
				require.Equal(t, "Your session has ended. Please sign in again.", errorDetail(t, res))

				res = c.do(http.MethodPost, "/api/auth/password-changes",
					`{"data":{"type":"password-changes","attributes":{"current_password":"wrong-one","password":"An0ther-Passw0rd"}}}`, laptop)
				require.Equal(t, http.StatusUnprocessableEntity, res.Code)
				require.Equal(t, "The current password is incorrect.", errorDetail(t, res))
			},
		},
		{
			name: "email verification: register emails a link, opening it verifies the email (once); a resend after that is a 409",
			run: func(t *testing.T, c apiClient) {
				before := len(mail.Sent()) // the mailer is shared by every case
				res := c.do(http.MethodPost, "/api/auth/register",
					`{"data":{"type":"users","attributes":{"name":"Jane","email":"verify@example.com","password":"`+testhelper.DefaultPassword+`"}}}`, "")
				require.Equal(t, http.StatusCreated, res.Code, "register: %v", res.Body)
				sent := mail.Sent()[before:]
				require.Len(t, sent, 1)
				require.Equal(t, "Verify your email", sent[0].Subject)
				_, token, _ := strings.Cut(sent[0].Text, "token=")

				access := c.login("verify@example.com")
				require.Equal(t, false, dataAttr(t, c.do(http.MethodGet, "/api/me", "", access), "email_verified"),
					"an unverified email doesn't block login")

				res = c.do(http.MethodPost, "/api/me/email-verification-requests", "", access)
				require.Equal(t, http.StatusAccepted, res.Code, "resend: %v", res.Body)
				require.Equal(t, "We've sent a new verification link.", res.Body["meta"].(map[string]any)["message"])
				resent := mail.Sent()[before+1]
				_, newToken, _ := strings.Cut(resent.Text, "token=")

				verify := func(tok string) apiResponse {
					return c.do(http.MethodPost, "/api/auth/email-verifications",
						`{"data":{"type":"email-verifications","attributes":{"token":"`+tok+`"}}}`, "")
				}
				res = verify(token)
				require.Equal(t, http.StatusUnprocessableEntity, res.Code, "a resend spends the earlier link")
				require.Equal(t, http.StatusNoContent, verify(newToken).Code)
				require.Equal(t, true, dataAttr(t, c.do(http.MethodGet, "/api/me", "", access), "email_verified"))
				require.Equal(t, http.StatusUnprocessableEntity, verify(newToken).Code, "a link works once")

				res = c.do(http.MethodPost, "/api/me/email-verification-requests",
					`{"data":{"type":"email-verification-requests"}}`, access)
				require.Equal(t, http.StatusConflict, res.Code)
				require.Equal(t, "Your email is already verified.", errorDetail(t, res))
			},
		},
		{
			name: "every /api request lands in the activity log once the recorder flushes, with the caller and the route",
			run: func(t *testing.T, c apiClient) {
				// The recorder is shared by every case: write what earlier cases
				// queued, then start from an empty table.
				drained, cancelDrain := context.WithCancel(context.Background())
				cancelDrain()
				activity.Run(drained)
				_, err := testDB.Exec(`TRUNCATE TABLE api_activity_logs`)
				require.NoError(t, err)

				userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
				token := c.login("jane@example.com")
				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", token).Code)
				require.Equal(t, http.StatusNotFound, c.do(http.MethodGet, "/api/nope?secret=1", "", "").Code)

				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				activity.Run(ctx) // drains and writes what was recorded, then returns

				rows, err := testDB.Query(`SELECT method, route, path, status, user_id, session_id IS NOT NULL FROM api_activity_logs ORDER BY id`)
				require.NoError(t, err)
				defer func() { _ = rows.Close() }()
				type row struct {
					method, route, path string
					status              int
					userID              sql.NullInt64
					hasSession          bool
				}
				var got []row
				for rows.Next() {
					var r row
					require.NoError(t, rows.Scan(&r.method, &r.route, &r.path, &r.status, &r.userID, &r.hasSession))
					got = append(got, r)
				}
				require.NoError(t, rows.Err())
				require.Equal(t, []row{
					{method: "POST", route: "/api/auth/login", path: "/api/auth/login", status: 200},
					{method: "GET", route: "/api/me", path: "/api/me", status: 200, userID: sql.NullInt64{Int64: userID, Valid: true}, hasSession: true},
					{method: "GET", route: "", path: "/api/nope", status: 404},
				}, got)
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
