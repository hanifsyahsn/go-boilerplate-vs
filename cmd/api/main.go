package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	adminusersvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user"
	adminusersqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user/sqlc"
	authloginsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login"
	authloginsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authlogoutsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout"
	authrefreshsvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh"
	authregistersvc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register"
	authregistersqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register/sqlc"
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
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/config"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/server"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"

	_ "github.com/lib/pq"
)

// Version is set at build time (-ldflags "-X main.Version=...").
var Version = "dev"

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

	conn, err := sql.Open(conf.DBDriver, conf.DBSource)
	if err != nil {
		logging.Fatal("open db failed", "err", err)
	}
	defer conn.Close()

	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(25)
	conn.SetConnMaxLifetime(30 * time.Minute)
	conn.SetConnMaxIdleTime(5 * time.Minute)

	if err = conn.PingContext(ctx); err != nil {
		logging.Fatal("ping db failed", "err", err)
	}

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

	// Queriers shared by more than one slice.
	authSharedQuerier := authsharedsqlc.New(conn)
	noteSharedQuerier := notesharedsqlc.New(conn)

	// Wiring per slice: sqlc.New(conn) → NewStore(conn) (tx slices only) →
	// NewService(...) → NewHandler(service).
	handlers := server.Handlers{
		Register: authregistersvc.NewHandler(authregistersvc.NewService(authregistersqlc.New(conn))),
		Login: authloginsvc.NewHandler(
			authloginsvc.NewService(authloginsqlc.New(conn), authSharedQuerier, tokenMaker,
				conf.AccessTokenDuration, conf.RefreshTokenDuration),
			cookies,
		),
		Refresh: authrefreshsvc.NewHandler(
			authrefreshsvc.NewService(authrefreshsvc.NewStore(conn), tokenMaker,
				conf.AccessTokenDuration, conf.RefreshTokenDuration),
			cookies,
		),
		Logout: authlogoutsvc.NewHandler(authlogoutsvc.NewService(authSharedQuerier, tokenMaker), cookies),

		Me: mesvc.NewHandler(mesvc.NewService(mesqlc.New(conn))),

		CreateNote: createnotesvc.NewHandler(createnotesvc.NewService(noteSharedQuerier, createnotesqlc.New(conn))),
		ListNote:   listnotesvc.NewHandler(listnotesvc.NewService(listnotesqlc.New(conn))),
		DetailNote: detailnotesvc.NewHandler(detailnotesvc.NewService(noteSharedQuerier)),
		UpdateNote: updatenotesvc.NewHandler(updatenotesvc.NewService(updatenotesvc.NewStore(conn))),
		DeleteNote: deletenotesvc.NewHandler(deletenotesvc.NewService(deletenotesvc.NewStore(conn))),

		AdminUser: adminusersvc.NewHandler(adminusersvc.NewService(adminusersqlc.New(conn))),
	}

	srv := server.NewServer(conf, Version, tokenMaker, middlewaresqlc.New(conn), handlers)
	if err = srv.Run(ctx); err != nil {
		logging.Fatal("server run failed", "err", err)
	}
}
