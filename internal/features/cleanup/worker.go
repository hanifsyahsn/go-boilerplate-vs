// Package cleanup deletes rows of no further use (decision 0005): expired
// refresh tokens, sessions left without tokens, expired one-time email
// tokens and activity-log rows past their retention. No endpoint; main runs
// the Worker in its background group.
package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/cleanup/sqlc"
)

const (
	// Interval is the time between two runs.
	Interval = 24 * time.Hour
	// firstRunDelay lets the service settle after a start (a deploy) first.
	firstRunDelay = time.Minute
	// batchSize rows at most per DELETE, so no run holds a long lock.
	batchSize = 10_000
)

// Result is how many rows a run deleted, per table.
type Result struct {
	RefreshTokens, AuthSessions, AuthTokens, ActivityLogs int64
}

// Worker deletes expired rows. Every replica runs it; the deletes are
// idempotent, so two replicas at once only share the work.
type Worker struct {
	querier           sqlc.Querier
	activityRetention time.Duration

	// Seams for tests.
	batchSize     int32
	firstRunDelay time.Duration
	interval      time.Duration
	now           func() time.Time
}

// NewWorker builds the worker; activityRetention 0 keeps the activity log
// forever.
func NewWorker(querier sqlc.Querier, activityRetention time.Duration) *Worker {
	return &Worker{
		querier:           querier,
		activityRetention: activityRetention,
		batchSize:         batchSize,
		firstRunDelay:     firstRunDelay,
		interval:          Interval,
		now:               time.Now,
	}
}

// Run waits firstRunDelay, runs once, then once every Interval, until ctx is
// done.
func (w *Worker) Run(ctx context.Context) {
	timer := time.NewTimer(w.firstRunDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			w.RunOnce(ctx)
			timer.Reset(w.interval)
		}
	}
}

// RunOnce deletes, table by table, in batches until a batch comes back short.
// One table's error is logged and the others still run; a done ctx stops it
// between batches.
func (w *Worker) RunOnce(ctx context.Context) Result {
	start := w.now()
	var res Result
	res.RefreshTokens = w.deleteAll(ctx, "refresh_tokens", w.querier.DeleteExpiredRefreshTokens)
	res.AuthSessions = w.deleteAll(ctx, "auth_sessions", w.querier.DeleteOrphanSessions)
	res.AuthTokens = w.deleteAll(ctx, "auth_tokens", w.querier.DeleteExpiredAuthTokens)
	if w.activityRetention > 0 {
		before := w.now().Add(-w.activityRetention)
		res.ActivityLogs = w.deleteAll(ctx, "api_activity_logs", func(ctx context.Context, n int32) (int64, error) {
			return w.querier.DeleteOldActivityLogs(ctx, sqlc.DeleteOldActivityLogsParams{Before: before, BatchSize: n})
		})
	}

	slog.InfoContext(ctx, "cleanup done", "refresh_tokens", res.RefreshTokens, "auth_sessions", res.AuthSessions,
		"auth_tokens", res.AuthTokens, "api_activity_logs", res.ActivityLogs, "duration_ms", w.now().Sub(start).Milliseconds())
	return res
}

// deleteAll repeats del until a batch deletes fewer than the batch size, an
// error (logged) or ctx is done; it returns the rows deleted.
func (w *Worker) deleteAll(ctx context.Context, table string, del func(context.Context, int32) (int64, error)) int64 {
	var total int64
	for ctx.Err() == nil {
		n, err := del(ctx, w.batchSize)
		if err != nil {
			slog.ErrorContext(ctx, "cleanup failed", "table", table, "err", err)
			return total
		}
		total += n
		if n < int64(w.batchSize) {
			return total
		}
	}
	return total
}
