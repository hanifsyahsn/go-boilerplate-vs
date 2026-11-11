package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	adminusersvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user"
	adminusersqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user/sqlc"
	authemailverificationsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverification"
	authemailverificationrequestsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverificationrequest"
	authemailverificationrequestsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverificationrequest/sqlc"
	authloginsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login"
	authloginsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authlogoutsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout"
	authlogoutsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout/sqlc"
	authpasswordchangesvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordchange"
	authpasswordchangesqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordchange/sqlc"
	authpasswordresetsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordreset"
	authpasswordresetrequestsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordresetrequest"
	authpasswordresetrequestsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordresetrequest/sqlc"
	authrefreshsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh"
	authregistersvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	mesvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me"
	mesqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me/sqlc"
	createnotesvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create"
	createnotesqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create/sqlc"
	deletenotesvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/delete"
	detailnotesvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/detail"
	listnotesvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list"
	listnotesqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list/sqlc"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	updatenotesvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/update"
	sessionlistsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/list"
	sessionlistsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/list/sqlc"
	sessionrevokesvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/revoke"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/config"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/server"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"

	_ "github.com/lib/pq"
)

// Version is set at build time (-ldflags "-X main.Version=...").
var Version = "dev"

// Rate limits: token buckets, in memory, one instance (see
// internal/platform/ratelimit).
var (
	// authIPRate caps register + login together per client IP. Generous on
	// purpose: an office behind one NAT shares an IP.
	authIPRate = ratelimit.Rate{Burst: 300, Per: 15 * time.Minute}
	// publicIPRate caps every public route but refresh / logout per client
	// IP: a flood guard only, generous because an office NAT or a mobile
	// carrier puts many users behind one IP.
	publicIPRate = ratelimit.Rate{Burst: 300, Per: time.Minute}
	// loginFailureRate caps failed logins per email: 5 at once, then one
	// more every 3 minutes.
	loginFailureRate = ratelimit.Rate{Burst: 5, Per: 15 * time.Minute}
	// loginIPFailureRate caps failed logins per client IP across every email
	// (credential stuffing tries each email once): 100 at once, then one more
	// every 3.6 minutes. Only failures count, so a NAT full of people who
	// type their password right never reaches it.
	loginIPFailureRate = ratelimit.Rate{Burst: 100, Per: 6 * time.Hour}
	// resetRecipientRate caps forgot-password emails per address, so the
	// endpoint can't flood an inbox through our SMTP.
	resetRecipientRate = ratelimit.Rate{Burst: 3, Per: time.Hour}
	// passwordChangeRate caps wrong current passwords per user (a stolen
	// access token can't brute-force the password): 5 at once, then one every
	// 3 minutes.
	passwordChangeRate = ratelimit.Rate{Burst: 5, Per: 15 * time.Minute}
	// verifyResendRate caps new verification links per user.
	verifyResendRate = ratelimit.Rate{Burst: 3, Per: time.Hour}
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conf, err := config.LoadConfig(".")
	if err != nil {
		logging.Fatal("load config failed", "err", err)
	}
	if err = logging.Setup(conf.LogFormat, conf.LogLevel); err != nil {
		logging.Fatal("setup logging failed", "err", err)
	}

	slog.InfoContext(ctx, "starting go_boilerplate_vs", "version", Version, "environment", conf.Environment)
	if len(conf.AllowedOrigins) == 0 {
		slog.WarnContext(ctx, "ALLOWED_ORIGINS is empty: CORS and the origin guard are off")
	}

	conn, err := sql.Open(conf.DBDriver, conf.DBSource)
	if err != nil {
		logging.Fatal("open db failed", "err", err)
	}
	defer func() { _ = conn.Close() }()

	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(25)
	conn.SetConnMaxLifetime(30 * time.Minute)
	conn.SetConnMaxIdleTime(5 * time.Minute)

	if err = conn.PingContext(ctx); err != nil {
		logging.Fatal("ping db failed", "err", err)
	}

	// Background work (workers, emails, the activity recorder) lives past the
	// signal: bgCtx is cancelled only after the server stopped (below), and
	// main waits for bg before the DB pool closes.
	bgCtx, stopBg := context.WithCancel(context.WithoutCancel(ctx))
	defer stopBg()
	var bg utility.Group
	_ = bgCtx // the first worker (decisions 0005, 0007, 0008, 0011) starts with bg.Go(bgCtx, ...)

	if err = validation.RegisterGinValidators(); err != nil {
		logging.Fatal("register validators failed", "err", err)
	}

	privateKey, err := jwttoken.LoadECPrivateKey(conf.ECPrivateKeyPath)
	if err != nil {
		logging.Fatal("load ec private key failed", "err", err)
	}
	publicKey, err := jwttoken.LoadECPublicKey(conf.ECPublicKeyPath)
	if err != nil {
		logging.Fatal("load ec public key failed", "err", err)
	}
	tokenMaker := jwttoken.NewTokenMakerES256(privateKey, publicKey, conf.ClientID, conf.TokenIssuer)

	cookies := authshared.CookieConfig{Secure: conf.CookieSecure, Domain: conf.CookieDomain}

	// Emails go out off the request path, in the background group main
	// waits for at shutdown. No SMTP_HOST (development only) logs them.
	var mail mailer.Mailer = mailer.NewLog()
	if conf.SMTPHost != "" {
		mail = mailer.NewSMTP(mailer.SMTPConfig{Host: conf.SMTPHost, Port: conf.SMTPPort,
			Username: conf.SMTPUsername, Password: conf.SMTPPassword, From: conf.SMTPFrom})
	} else {
		slog.WarnContext(ctx, "SMTP_HOST is empty: emails are logged, not sent")
	}
	emailer := authshared.Emailer{Mailer: mail, Go: bg.Go, AppBaseURL: conf.AppBaseURL}

	// Queriers shared by more than one slice.
	authSharedQuerier := authsharedsqlc.New(conn)
	noteSharedQuerier := notesharedsqlc.New(conn)

	// Wiring per slice: sqlc.New(conn) → NewStore(conn) (tx slices only) →
	// NewService(...) → NewHandler(service).
	handlers := server.Handlers{
		Register: authregistersvc.NewHandler(authregistersvc.NewService(authregistersvc.NewStore(conn), emailer)),
		Login: authloginsvc.NewHandler(
			authloginsvc.NewService(authloginsqlc.New(conn), authSharedQuerier, tokenMaker,
				conf.AccessTokenDuration, conf.RefreshTokenDuration, conf.UntrustedSessionDuration,
				authloginsvc.Limiters{
					Email: ratelimit.NewMemory(loginFailureRate),
					IP:    ratelimit.NewMemory(loginIPFailureRate),
				}),
			cookies,
		),
		Refresh: authrefreshsvc.NewHandler(
			authrefreshsvc.NewService(authrefreshsvc.NewStore(conn), tokenMaker,
				conf.AccessTokenDuration, conf.RefreshTokenDuration),
			cookies,
		),
		PasswordResetRequest: authpasswordresetrequestsvc.NewHandler(authpasswordresetrequestsvc.NewService(
			authpasswordresetrequestsqlc.New(conn), authpasswordresetrequestsvc.NewStore(conn),
			ratelimit.NewMemory(resetRecipientRate), emailer)),
		EmailVerification: authemailverificationsvc.NewHandler(authemailverificationsvc.NewService(authemailverificationsvc.NewStore(conn))),
		EmailVerificationRequest: authemailverificationrequestsvc.NewHandler(authemailverificationrequestsvc.NewService(
			authemailverificationrequestsqlc.New(conn), authemailverificationrequestsvc.NewStore(conn),
			ratelimit.NewMemory(verifyResendRate), emailer)),
		PasswordChange: authpasswordchangesvc.NewHandler(authpasswordchangesvc.NewService(authpasswordchangesqlc.New(conn),
			authpasswordchangesvc.NewStore(conn), ratelimit.NewMemory(passwordChangeRate), emailer)),
		PasswordReset: authpasswordresetsvc.NewHandler(authpasswordresetsvc.NewService(authpasswordresetsvc.NewStore(conn), emailer)),
		Logout:        authlogoutsvc.NewHandler(authlogoutsvc.NewService(authlogoutsqlc.New(conn), authSharedQuerier, tokenMaker), cookies),

		Me: mesvc.NewHandler(mesvc.NewService(mesqlc.New(conn))),

		ListSessions:  sessionlistsvc.NewHandler(sessionlistsvc.NewService(sessionlistsqlc.New(conn))),
		RevokeSession: sessionrevokesvc.NewHandler(sessionrevokesvc.NewService(sessionrevokesvc.NewStore(conn)), cookies),

		CreateNote: createnotesvc.NewHandler(createnotesvc.NewService(noteSharedQuerier, createnotesqlc.New(conn))),
		ListNote:   listnotesvc.NewHandler(listnotesvc.NewService(listnotesqlc.New(conn))),
		DetailNote: detailnotesvc.NewHandler(detailnotesvc.NewService(noteSharedQuerier)),
		UpdateNote: updatenotesvc.NewHandler(updatenotesvc.NewService(updatenotesvc.NewStore(conn))),
		DeleteNote: deletenotesvc.NewHandler(deletenotesvc.NewService(deletenotesvc.NewStore(conn))),

		AdminUser: adminusersvc.NewHandler(adminusersvc.NewService(adminusersqlc.New(conn), adminusersvc.NewStore(conn))),
	}

	gin.SetMode(server.GinMode(conf.Environment))
	srv, err := server.NewServer(conf, Version, tokenMaker, middlewaresqlc.New(conn), handlers, server.Security{
		TrustedProxies: conf.TrustedProxies,
		AuthIPLimit:    ratelimit.NewMemory(authIPRate),
		PublicIPLimit:  ratelimit.NewMemory(publicIPRate),
		AllowedOrigins: conf.AllowedOrigins,
	})
	if err != nil {
		logging.Fatal("build server failed", "err", err)
	}
	runErr := srv.Run(ctx) // returns once in-flight requests are drained
	// Only now: a request that finished during the drain has queued its
	// background work, which gets the cancel and flushes before the DB pool
	// (deferred above) closes.
	stopBg()
	bg.Wait()
	if runErr != nil {
		logging.Fatal("server run failed", "err", runErr)
	}
}
