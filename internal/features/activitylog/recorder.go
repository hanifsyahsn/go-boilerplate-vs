// Package activitylog writes the API activity log (decision 0008): the
// middleware hands each request to the Recorder, which queues it and writes
// batches off the request path. No endpoint reads it; it is queried by hand.
package activitylog

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/activitylog/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/notifier"
)

const (
	// QueueSize is how many requests wait for a write; past it Record drops.
	QueueSize = 4096
	// BatchSize rows are written in one statement at most.
	BatchSize = 200
	// FlushInterval writes a partial batch at least this often.
	FlushInterval = 2 * time.Second
	// flushTimeout bounds one write (and the last one, at shutdown).
	flushTimeout = 5 * time.Second
)

// Recorder queues activity and writes it in batches. Record never blocks; Run
// does the writing and, when its ctx is done, drains the queue and writes it
// before returning (main cancels that ctx only after the HTTP server drained
// its requests, then waits for Run — the shutdown flush).
type Recorder struct {
	querier sqlc.Querier
	queue   chan middleware.Activity
	dropped atomic.Int64
	alerts  notifier.Notifier

	// batchSize / interval are seams for tests.
	batchSize int
	interval  time.Duration
}

func NewRecorder(querier sqlc.Querier) *Recorder {
	return &Recorder{
		querier:   querier,
		queue:     make(chan middleware.Activity, QueueSize),
		alerts:    notifier.NewNoop(),
		batchSize: BatchSize,
		interval:  FlushInterval,
	}
}

// SetNotifier makes a failed write an ops alert too (decision 0011).
func (r *Recorder) SetNotifier(n notifier.Notifier) { r.alerts = n }

// Record queues a; a full queue drops it (counted, logged at the next write).
func (r *Recorder) Record(a middleware.Activity) {
	select {
	case r.queue <- a:
	default:
		r.dropped.Add(1)
	}
}

// Run writes a batch when it reaches the batch size or every flush interval.
// When ctx is done it drains what is queued at that moment, writes it and
// returns. The queue is never closed (Record may still be called).
func (r *Recorder) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	batch := make([]middleware.Activity, 0, r.batchSize)
	flush := func() {
		if len(batch) > 0 {
			r.write(ctx, batch)
			batch = batch[:0]
		}
	}

	for {
		select {
		case a := <-r.queue:
			batch = append(batch, a)
			if len(batch) >= r.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			for {
				select {
				case a := <-r.queue:
					batch = append(batch, a)
					if len(batch) >= r.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// write inserts one batch in one statement, on a ctx that outlives the
// cancelled one at shutdown but is bounded by flushTimeout. A failed write
// is logged and its rows dropped (no retry).
func (r *Recorder) write(ctx context.Context, batch []middleware.Activity) {
	if n := r.dropped.Swap(0); n > 0 {
		slog.WarnContext(ctx, "activity log: queue full, rows dropped", "dropped", n)
	}

	p := sqlc.InsertActivityLogsParams{
		OccurredAt: make([]time.Time, len(batch)), RequestID: make([]string, len(batch)),
		UserID: make([]int64, len(batch)), SessionID: make([]uuid.UUID, len(batch)),
		Method: make([]string, len(batch)), Route: make([]string, len(batch)), Path: make([]string, len(batch)),
		Status: make([]int32, len(batch)), LatencyMs: make([]int32, len(batch)), ResponseBytes: make([]int32, len(batch)),
		ClientIp: make([]string, len(batch)), UserAgent: make([]string, len(batch)),
	}
	for i, a := range batch {
		p.OccurredAt[i], p.RequestID[i], p.UserID[i], p.SessionID[i] = a.OccurredAt, a.RequestID, a.UserID, a.SessionID
		p.Method[i], p.Route[i], p.Path[i] = a.Method, a.Route, a.Path
		p.Status[i] = clampInt32(int64(a.Status))
		p.LatencyMs[i] = clampInt32(a.Latency.Milliseconds())
		p.ResponseBytes[i] = clampInt32(int64(a.ResponseBytes))
		p.ClientIp[i], p.UserAgent[i] = a.ClientIP, a.UserAgent
	}

	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
	defer cancel()
	if err := r.querier.InsertActivityLogs(wctx, p); err != nil {
		slog.ErrorContext(ctx, "activity log: write failed", "rows", len(batch), "err", err)
		r.alerts.Notify(ctx, notifier.Alert{Level: notifier.LevelError, Title: "activity log: write failed",
			Text: fmt.Sprintf("%d rows dropped: %v", len(batch), err)})
	}
}

// clampInt32 fits n into an integer column: negative is 0, past MaxInt32 is
// MaxInt32.
func clampInt32(n int64) int32 {
	return int32(min(max(n, 0), math.MaxInt32)) // #nosec G115 -- clamped to the int32 range just here
}
