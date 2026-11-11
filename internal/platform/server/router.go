package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	adminuser "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user"
	authemailverification "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverification"
	authemailverificationrequest "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverificationrequest"
	authlogin "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login"
	authlogout "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout"
	authpasswordchange "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordchange"
	authpasswordreset "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordreset"
	authpasswordresetrequest "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordresetrequest"
	authrefresh "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh"
	authregister "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me"
	createnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create"
	deletenote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/delete"
	detailnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/detail"
	listnote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list"
	updatenote "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/update"
	sessionlist "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/list"
	sessionrevoke "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/revoke"
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

	PasswordResetRequest     authpasswordresetrequest.H
	PasswordReset            authpasswordreset.H
	PasswordChange           authpasswordchange.H
	EmailVerification        authemailverification.H
	EmailVerificationRequest authemailverificationrequest.H

	Me me.H

	ListSessions  sessionlist.H
	RevokeSession sessionrevoke.H

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
	AuthIPLimit ratelimit.Limiter
	// PublicIPLimit caps every public route per client IP, a flood guard
	// generous enough for an office NAT; refresh and logout are left out
	// (they run on every open tab). Required.
	PublicIPLimit ratelimit.Limiter
	// AllowedOrigins (ALLOWED_ORIGINS, normalised) are the browser origins
	// CORS answers and the origin guard lets write. Empty turns both off.
	AllowedOrigins []string
}

// The limiters' key prefixes: one bucket per name and client IP.
const (
	authIPLimitName   = "auth"
	publicIPLimitName = "public"
)

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

// NewRouter mounts three groups under /api, all behind the origin guard
// (middleware.OriginGuard: a write from an unlisted Origin is a 403), then
// JSON:API content negotiation (middleware.JSONAPIMediaType):
//
//	public   no auth: refresh / logout with no IP limit (every open tab
//	         calls them); every other public route behind the general per-IP
//	         limit (Security.PublicIPLimit), and register / login also behind
//	         the auth one (Security.AuthIPLimit)
//	authed   AccessTokenMiddleware (any active user)
//	/admin   AccessTokenMiddleware + RequireRole(admin)
//
// Every route (404 / 405 / panic included) runs RequestID → AccessLog →
// recovery → CORS first, so every response carries X-Request-ID and gets one
// access log record, and a browser preflight (an OPTIONS no route declares)
// is answered by CORS instead of a 405.
//
// There's also an unauthenticated GET /healthz liveness probe that reports the build
// version. Unknown routes (404), a wrong method (405) and a panic (500) answer
// with JSON:API errors too.
//
// It fails on a Security it can't apply: a TrustedProxies entry gin rejects,
// or no AuthIPLimit / PublicIPLimit.
func NewRouter(version string, tokenMaker jwttoken.M, accessStatus middleware.AccessStatusReader, h Handlers, sec Security) (*gin.Engine, error) {
	if sec.AuthIPLimit == nil {
		return nil, errors.New("server.NewRouter(): Security.AuthIPLimit is required")
	}
	if sec.PublicIPLimit == nil {
		return nil, errors.New("server.NewRouter(): Security.PublicIPLimit is required")
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
	}), middleware.CORS(sec.AllowedOrigins))
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

	// The origin guard first: a cross-site write is refused before anything
	// else looks at it.
	api := r.Group("/api", middleware.OriginGuard(sec.AllowedOrigins), middleware.JSONAPIMediaType())

	// Refresh and logout work off the refresh cookie and run on every open
	// tab, so they aren't IP-limited (an office or a mobile carrier puts many
	// users behind one IP, and a forged token fails the signature check before
	// the DB). Every other public route is behind the general per-IP limit,
	// a flood guard; register and login also behind the auth one, on top of
	// login's own failure limits. A new public route goes into guarded (or
	// limited when it touches credentials or sends email).
	public := api.Group("")
	{
		authrefresh.Route(public, h.Refresh)
		authlogout.Route(public, h.Logout)
	}
	guarded := public.Group("")
	guarded.Use(middleware.RateLimitByIP(sec.PublicIPLimit, publicIPLimitName))
	limited := guarded.Group("")
	limited.Use(middleware.RateLimitByIP(sec.AuthIPLimit, authIPLimitName))
	{
		authregister.Route(limited, h.Register)
		authlogin.Route(limited, h.Login)
		authpasswordresetrequest.Route(limited, h.PasswordResetRequest)
		authpasswordreset.Route(limited, h.PasswordReset)
		authemailverification.Route(limited, h.EmailVerification)
	}

	authed := api.Group("")
	authed.Use(middleware.AccessTokenMiddleware(tokenMaker, accessStatus))
	{
		me.Route(authed, h.Me)
		authpasswordchange.Route(authed, h.PasswordChange)
		authemailverificationrequest.Route(authed, h.EmailVerificationRequest)
		sessionlist.Route(authed, h.ListSessions)
		sessionrevoke.Route(authed, h.RevokeSession)
		createnote.Route(authed, h.CreateNote)
		listnote.Route(authed, h.ListNote)
		detailnote.Route(authed, h.DetailNote)
		updatenote.Route(authed, h.UpdateNote)
		deletenote.Route(authed, h.DeleteNote)
	}

	admin := api.Group("/admin")
	admin.Use(
		middleware.AccessTokenMiddleware(tokenMaker, accessStatus),
		middleware.RequireRole(enum.RoleAdmin),
	)
	{
		adminuser.Route(admin, h.AdminUser)
	}

	return r, nil
}
