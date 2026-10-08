package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/config"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
)

// shutdownTimeout is how long in-flight requests get to finish on shutdown.
const shutdownTimeout = 5 * time.Second

type Server struct {
	httpServer *http.Server
}

func NewServer(conf config.Config, version string, tokenMaker jwttoken.M, userStatus middleware.UserStatusReader, handlers Handlers) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              conf.ServerAddress,
			Handler:           NewRouter(version, tokenMaker, userStatus, handlers),
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Run serves until ctx is cancelled (SIGINT / SIGTERM in main), then shuts
// down gracefully. It returns the listener's error if the server couldn't
// start.
func (server *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		logger.Infof("server running on %s", server.httpServer.Addr)
		if err := server.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Errorf("server forced to shutdown: %v", err)
		return err
	}

	logger.Info("server exited gracefully")
	return nil
}
