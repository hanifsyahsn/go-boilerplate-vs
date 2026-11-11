package activitylog

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/activitylog/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
	"github.com/stretchr/testify/require"
)

// fakeQuerier records each batch's size and fails the first `fail` writes.
type fakeQuerier struct {
	mu      sync.Mutex
	batches []sqlc.InsertActivityLogsParams
	fail    int
	written chan int
}

func (f *fakeQuerier) InsertActivityLogs(_ context.Context, p sqlc.InsertActivityLogsParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail > 0 {
		f.fail--
		f.written <- -len(p.Method)
		return errors.New("db down")
	}
	f.batches = append(f.batches, p)
	f.written <- len(p.Method)
	return nil
}

// syncBuffer is a bytes.Buffer safe for the logger of several goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func activity(i int) middleware.Activity {
	return middleware.Activity{OccurredAt: time.Now(), Method: "GET", Path: "/api/x", Status: 200 + i%2, Latency: time.Duration(i) * time.Millisecond}
}

func TestRecorder_U(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T, r *Recorder, q *fakeQuerier, logs *syncBuffer)
	}{
		{name: "a full batch is written at once, without waiting for the interval", run: func(t *testing.T, r *Recorder, q *fakeQuerier, _ *syncBuffer) {
			for i := range 3 {
				r.Record(activity(i))
			}
			select {
			case n := <-q.written:
				require.Equal(t, 3, n)
			case <-time.After(time.Second):
				t.Fatal("the full batch wasn't written")
			}
		}},
		{name: "a partial batch is written after the interval", run: func(t *testing.T, r *Recorder, q *fakeQuerier, _ *syncBuffer) {
			r.Record(activity(0))
			select {
			case n := <-q.written:
				require.Equal(t, 1, n)
			case <-time.After(2 * time.Second):
				t.Fatal("the partial batch wasn't written on the interval")
			}
		}},
		{name: "a failed write is logged with its row count and the recorder goes on", run: func(t *testing.T, r *Recorder, q *fakeQuerier, logs *syncBuffer) {
			q.mu.Lock()
			q.fail = 1
			q.mu.Unlock()
			for i := range 3 {
				r.Record(activity(i))
			}
			require.Equal(t, -3, <-q.written)
			for i := range 3 {
				r.Record(activity(i))
			}
			require.Equal(t, 3, <-q.written, "the next batch is written")
			require.Contains(t, logs.String(), `"msg":"activity log: write failed"`)
			require.Contains(t, logs.String(), `"rows":3`)
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := &syncBuffer{}
			l, err := logging.New(logs, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			q := &fakeQuerier{written: make(chan int, 10)}
			r := NewRecorder(q)
			r.batchSize, r.interval = 3, 100*time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { r.Run(ctx); close(done) }()
			defer func() { cancel(); <-done }()

			tc.run(t, r, q, logs)
		})
	}
}

// TestRecorder_QueueFull_U: Record never blocks; past the queue it drops and
// the next write logs how many.
func TestRecorder_QueueFull_U(t *testing.T) {
	testCases := []struct {
		name        string
		queue       int
		records     int
		wantWritten int
		wantDropped string
	}{
		{name: "a full queue drops and the next write logs the count", queue: 2, records: 5, wantWritten: 2, wantDropped: `"dropped":3`},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := &syncBuffer{}
			l, err := logging.New(logs, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			q := &fakeQuerier{written: make(chan int, 10)}
			r := NewRecorder(q)
			r.queue = make(chan middleware.Activity, tc.queue)
			for i := range tc.records { // Run isn't started: nothing drains, and Record must not block
				r.Record(activity(i))
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r.Run(ctx) // drains and writes, then returns

			require.Equal(t, tc.wantWritten, <-q.written)
			require.Contains(t, logs.String(), `"msg":"activity log: queue full, rows dropped"`)
			require.Contains(t, logs.String(), tc.wantDropped)
		})
	}
}

// TestRecorder_Drain_U: on cancel Run writes everything queued, in batches,
// on a ctx that isn't the cancelled one, then returns.
func TestRecorder_Drain_U(t *testing.T) {
	testCases := []struct {
		name        string
		records     int
		batchSize   int
		wantBatches []int
	}{
		{name: "7 queued rows with a batch of 3 are written as 3, 3, 1 at shutdown", records: 7, batchSize: 3, wantBatches: []int{3, 3, 1}},
		{name: "nothing queued: no write", records: 0, batchSize: 3},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQuerier{written: make(chan int, 10)}
			r := NewRecorder(q)
			r.batchSize = tc.batchSize
			for i := range tc.records {
				r.Record(activity(i))
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r.Run(ctx)

			var got []int
			for _, b := range q.batches {
				got = append(got, len(b.Method))
			}
			require.Equal(t, tc.wantBatches, got)
		})
	}
}

func TestClampInt32_U(t *testing.T) {
	testCases := []struct {
		name string
		n    int64
		want int32
	}{
		{name: "in range: kept", n: 42, want: 42},
		{name: "negative (nothing written): 0", n: -1, want: 0},
		{name: "past int32: MaxInt32", n: math.MaxInt64, want: math.MaxInt32},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, clampInt32(tc.n))
		})
	}
}
