package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/activitylog"
	activitylogsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/activitylog/sqlc"
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
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/cleanup"
	cleanupsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/cleanup/sqlc"
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
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/notifier"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/server"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/redis/go-redis/v9"

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

	// Ops alerts (decision 0011): Telegram when configured, else none. Every
	// recovered panic (a request's or a background goroutine's) is one. The
	// sender has its own ctx and group, stopped after bg (stopBackground): a
	// job still finishing, e.g. a failed last activity-log flush, can alert.
	alertCtx, stopAlerts := context.WithCancel(context.WithoutCancel(ctx))
	defer stopAlerts()
	var alertSender utility.Group
	var alerts notifier.Notifier = notifier.NewNoop()
	if conf.TelegramBotToken != "" {
		telegram := notifier.NewTelegram(conf.TelegramBotToken, conf.TelegramChatID, conf.Environment, "go_boilerplate_vs")
		alertSender.Go(alertCtx, telegram.Run)
		alerts = telegram
	}
	utility.SetPanicHook(func(ctx context.Context, recovered any) {
		alerts.Notify(ctx, notifier.Alert{Level: notifier.LevelError, Title: "panic", Text: fmt.Sprint(recovered),
			RequestID: requestid.FromContext(ctx)})
	})

	// The API activity log: queued by the middleware, written in batches by
	// the recorder, flushed at shutdown (decision 0008).
	activity := activitylog.NewRecorder(activitylogsqlc.New(conn))
	activity.SetNotifier(alerts)
	bg.Go(bgCtx, activity.Run)
	// Expired tokens, dead sessions, old activity rows: a minute after start,
	// then daily, on every replica (the deletes are idempotent).
	cleaner := cleanup.NewWorker(cleanupsqlc.New(conn), conf.ActivityLogRetention)
	cleaner.SetNotifier(alerts)
	bg.Go(bgCtx, cleaner.Run)

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

	// Rate limits: in memory, or in Redis when REDIS_URL is set so every
	// replica counts together (decision 0003). Auth limits fall back to
	// memory when Redis is down (never fail open); the general per-IP limit
	// lets requests through instead.
	limits, rdb := newLimiters(ctx, conf.RedisURL)

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
				authloginsvc.Limiters{Email: limits.loginEmail, IP: limits.loginIP}),
			cookies,
		),
		Refresh: authrefreshsvc.NewHandler(
			authrefreshsvc.NewService(authrefreshsvc.NewStore(conn), tokenMaker,
				conf.AccessTokenDuration, conf.RefreshTokenDuration),
			cookies,
		),
		PasswordResetRequest: authpasswordresetrequestsvc.NewHandler(authpasswordresetrequestsvc.NewService(
			authpasswordresetrequestsqlc.New(conn), authSharedQuerier, authpasswordresetrequestsvc.NewStore(conn),
			limits.resetRecipient, emailer)),
		EmailVerification: authemailverificationsvc.NewHandler(authemailverificationsvc.NewService(authemailverificationsvc.NewStore(conn))),
		EmailVerificationRequest: authemailverificationrequestsvc.NewHandler(authemailverificationrequestsvc.NewService(
			authemailverificationrequestsqlc.New(conn), authemailverificationrequestsvc.NewStore(conn),
			limits.verifyResend, emailer)),
		PasswordChange: authpasswordchangesvc.NewHandler(authpasswordchangesvc.NewService(authpasswordchangesqlc.New(conn),
			authpasswordchangesvc.NewStore(conn), limits.passwordChange, emailer)),
		PasswordReset: authpasswordresetsvc.NewHandler(authpasswordresetsvc.NewService(authpasswordresetsvc.NewStore(conn), emailer)),
		Logout:        authlogoutsvc.NewHandler(authlogoutsvc.NewService(authlogoutsqlc.New(conn), authlogoutsvc.NewStore(conn), tokenMaker), cookies),

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
		AuthIPLimit:    limits.authIP,
		PublicIPLimit:  limits.publicIP,
		AllowedOrigins: conf.AllowedOrigins,

		ActivityRecorder: activity,
	})
	if err != nil {
		logging.Fatal("build server failed", "err", err)
	}
	runErr := srv.Run(ctx) // returns once in-flight requests are drained
	// Only now: a request that finished during the drain has queued its
	// background work, which gets the cancel and flushes before the DB pool
	// (deferred above) closes.
	stopBackground(stopBg, &bg, stopAlerts, &alertSender)
	if rdb != nil {
		_ = rdb.Close() // after every request and background job is done with it
	}
	if runErr != nil {
		logging.Fatal("server run failed", "err", runErr)
	}
}

// stopBackground ends the background work once the server has drained: the
// jobs first (they flush what the last requests queued, and a failure there
// may raise an alert), then the alert sender, which sends what is queued
// before it returns, so those last alerts still go out.
func stopBackground(stopJobs context.CancelFunc, jobs *utility.Group, stopAlerts context.CancelFunc, alertSender *utility.Group) {
	stopJobs()
	jobs.Wait()
	stopAlerts()
	alertSender.Wait()
}

// limiters are every rate limit of the API.
type limiters struct {
	authIP, publicIP               ratelimit.Limiter
	loginEmail, loginIP            ratelimit.Limiter
	resetRecipient, passwordChange ratelimit.Limiter
	verifyResend                   ratelimit.Limiter
}

// newLimiters builds the limits in memory, or in Redis when redisURL is set
// (the client is returned to be closed at shutdown). An unreachable Redis at
// start is only a warning: the fallbacks cover it until it comes back.
func newLimiters(ctx context.Context, redisURL string) (limiters, *redis.Client) {
	if redisURL == "" {
		return limiters{
			authIP: ratelimit.NewMemory(authIPRate), publicIP: ratelimit.NewMemory(publicIPRate),
			loginEmail: ratelimit.NewMemory(loginFailureRate), loginIP: ratelimit.NewMemory(loginIPFailureRate),
			resetRecipient: ratelimit.NewMemory(resetRecipientRate), passwordChange: ratelimit.NewMemory(passwordChangeRate),
			verifyResend: ratelimit.NewMemory(verifyResendRate),
		}, nil
	}

	opts, err := redis.ParseURL(redisURL) // checked by config.Validate
	if err != nil {
		logging.Fatal("parse REDIS_URL failed")
	}
	rdb := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err = rdb.Ping(pingCtx).Err(); err != nil {
		slog.WarnContext(ctx, "redis unreachable at start, limits use their fallback", "err", err)
	}

	auth := func(rate ratelimit.Rate, name string) ratelimit.Limiter {
		return ratelimit.WithFallback(ratelimit.NewRedis(rdb, rate, name), ratelimit.NewMemory(rate), name)
	}
	return limiters{
		authIP:         auth(authIPRate, "auth"),
		publicIP:       ratelimit.FailOpen(ratelimit.NewRedis(rdb, publicIPRate, "public"), "public"),
		loginEmail:     auth(loginFailureRate, "login"),
		loginIP:        auth(loginIPFailureRate, "loginip"),
		resetRecipient: auth(resetRecipientRate, "reset"),
		passwordChange: auth(passwordChangeRate, "pwchange"),
		verifyResend:   auth(verifyResendRate, "verify"),
	}, rdb
}
