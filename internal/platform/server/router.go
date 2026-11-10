package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
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
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// Handlers is every feature slice's handler, wired in cmd/api/main.go. A new
// slice adds its field here and one Route call in NewRouter.
type Handlers struct {
	Register authregister.H
	Login    authlogin.H
	Refresh  authrefresh.H
	Logout   authlogout.H

	Me me.H

	CreateNote createnote.H
	ListNote   listnote.H
	DetailNote detailnote.H
	UpdateNote updatenote.H
	DeleteNote deletenote.H

	AdminUser adminuser.H
}

// Security is the router's base protection, wired in cmd/api/main.go.
type Security struct {
	// TrustedProxies (TRUSTED_PROXIES) are the only peers whose
	// X-Forwarded-For / X-Real-IP set c.ClientIP(); empty trusts none, so a
	// client can't spoof its IP past the per-IP limit.
	TrustedProxies []string
	// AuthIPLimit caps register / login per client IP. Required.
	AuthIPLimit *ratelimit.Limiter
}

// authIPLimitName keys the auth routes' counters in AuthIPLimit.
const authIPLimitName = "auth"

// EnvironmentDevelopment is the ENVIRONMENT value that keeps gin in debug mode.
const EnvironmentDevelopment = "development"

// GinMode is the gin mode for an ENVIRONMENT: debug (route list, gin's
// warnings on stdout) only in development, release everywhere else so a
// deployed server doesn't print its routes or run gin's debug paths.
func GinMode(environment string) string {
	if environment == EnvironmentDevelopment {
		return gin.DebugMode
	}
	return gin.ReleaseMode
}

// NewRouter mounts three groups under /api, all behind JSON:API content
// negotiation (middleware.JSONAPIMediaType):
//
//	public   no auth (register / login / refresh / logout); register and
//	         login also behind the per-IP limit (Security.AuthIPLimit)
//	authed   AccessTokenMiddleware (any active user)
//	/admin   AccessTokenMiddleware + RequireRole(admin)
//
// Every route (404 / 405 / panic included) runs RequestID → AccessLog →
// recovery first, so every response carries X-Request-ID and gets one access
// log record.
//
// There's also an unauthenticated GET /healthz liveness probe that reports the build
// version. Unknown routes (404), a wrong method (405) and a panic (500) answer
// with JSON:API errors too.
//
// It fails on a Security it can't apply: a TrustedProxies entry gin rejects,
// or no AuthIPLimit.
func NewRouter(version string, tokenMaker jwttoken.M, userStatus middleware.UserStatusReader, h Handlers, sec Security) (*gin.Engine, error) {
	if sec.AuthIPLimit == nil {
		return nil, errors.New("server.NewRouter(): Security.AuthIPLimit is required")
	}

	r := gin.New()
	// gin trusts every proxy by default, which lets any client set its own
	// IP through X-Forwarded-For. nil trusts none.
	var trusted []string
	if len(sec.TrustedProxies) > 0 {
		trusted = sec.TrustedProxies
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		return nil, fmt.Errorf("server.NewRouter(): trusted proxies: %w", err)
	}
	r.Use(middleware.RequestID(), middleware.AccessLog(), gin.CustomRecovery(func(c *gin.Context, recovered any) {
		jsonapi.WriteError(c.Writer, apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("panic: %v", recovered))
		c.Abort()
	}))
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) {
		jsonapi.WriteError(c.Writer, apperror.CodeNotFound, "That resource doesn't exist.", nil)
	})
	r.NoMethod(func(c *gin.Context) {
		jsonapi.WriteError(c.Writer, apperror.CodeMethodNotAllowed,
			fmt.Sprintf("%s isn't allowed on this resource.", c.Request.Method), nil)
	})

	r.GET("/healthz", func(c *gin.Context) {
		jsonapi.WriteMeta(c.Writer, c.Request, http.StatusOK, gin.H{"status": "up", "version": version})
	})

	api := r.Group("/api", middleware.JSONAPIMediaType())

	// Refresh and logout work off the refresh cookie and run on every open
	// tab, so they aren't IP-limited (an office shares one IP); register and
	// login are, on top of login's own per-email failure limit.
	public := api.Group("")
	{
		authrefresh.Route(public, h.Refresh)
		authlogout.Route(public, h.Logout)
	}
	limited := public.Group("")
	limited.Use(middleware.RateLimitByIP(sec.AuthIPLimit, authIPLimitName))
	{
		authregister.Route(limited, h.Register)
		authlogin.Route(limited, h.Login)
	}

	authed := api.Group("")
	authed.Use(middleware.AccessTokenMiddleware(tokenMaker, userStatus))
	{
		me.Route(authed, h.Me)
		createnote.Route(authed, h.CreateNote)
		listnote.Route(authed, h.ListNote)
		detailnote.Route(authed, h.DetailNote)
		updatenote.Route(authed, h.UpdateNote)
		deletenote.Route(authed, h.DeleteNote)
	}

	admin := api.Group("/admin")
	admin.Use(
		middleware.AccessTokenMiddleware(tokenMaker, userStatus),
		middleware.RequireRole(enum.RoleAdmin),
	)
	{
		adminuser.Route(admin, h.AdminUser)
	}

	return r, nil
}
