package cleanup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/cleanup/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/notifier"
	"github.com/stretchr/testify/require"
)

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

func records(t *testing.T, logs string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		out = append(out, rec)
	}
	return out
}

func TestWorker_RunOnce_U(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		retention time.Duration
		stub      func(q *sqlc.MockQuerier)
		want      Result
		wantLogs  []string // messages, in order
	}{
		{
			name: "each table is deleted in batches until a short one, and the counts are logged", retention: time.Hour,
			stub: func(q *sqlc.MockQuerier) {
				gomock.InOrder(
					q.EXPECT().DeleteExpiredRefreshTokens(gomock.Any(), int32(2)).Return(int64(2), nil),
					q.EXPECT().DeleteExpiredRefreshTokens(gomock.Any(), int32(2)).Return(int64(1), nil),
					q.EXPECT().DeleteOrphanSessions(gomock.Any(), int32(2)).Return(int64(0), nil),
					q.EXPECT().DeleteExpiredAuthTokens(gomock.Any(), int32(2)).Return(int64(2), nil),
					q.EXPECT().DeleteExpiredAuthTokens(gomock.Any(), int32(2)).Return(int64(0), nil),
					q.EXPECT().DeleteOldActivityLogs(gomock.Any(), sqlc.DeleteOldActivityLogsParams{Before: now.Add(-time.Hour), BatchSize: 2}).
						Return(int64(1), nil),
				)
			},
			want:     Result{RefreshTokens: 3, AuthTokens: 2, ActivityLogs: 1},
			wantLogs: []string{"cleanup done"},
		},
		{
			name: "a retention of 0 keeps every activity row", retention: 0,
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().DeleteExpiredRefreshTokens(gomock.Any(), gomock.Any()).Return(int64(0), nil)
				q.EXPECT().DeleteOrphanSessions(gomock.Any(), gomock.Any()).Return(int64(0), nil)
				q.EXPECT().DeleteExpiredAuthTokens(gomock.Any(), gomock.Any()).Return(int64(0), nil)
			},
			wantLogs: []string{"cleanup done"},
		},
		{
			name: "one table's error is logged and the others still run", retention: time.Hour,
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().DeleteExpiredRefreshTokens(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
				q.EXPECT().DeleteOrphanSessions(gomock.Any(), gomock.Any()).Return(int64(1), nil)
				q.EXPECT().DeleteExpiredAuthTokens(gomock.Any(), gomock.Any()).Return(int64(0), nil)
				q.EXPECT().DeleteOldActivityLogs(gomock.Any(), gomock.Any()).Return(int64(0), nil)
			},
			want:     Result{AuthSessions: 1},
			wantLogs: []string{"cleanup failed", "cleanup done"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := &syncBuffer{}
			l, err := logging.New(logs, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tc.stub(q)
			w := NewWorker(q, tc.retention)
			w.batchSize = 2
			w.now = func() time.Time { return now }

			require.Equal(t, tc.want, w.RunOnce(context.Background()))
			recs := records(t, logs.String())
			var msgs []string
			for _, r := range recs {
				msgs = append(msgs, r["msg"].(string))
			}
			require.Equal(t, tc.wantLogs, msgs)
			done := recs[len(recs)-1]
			require.InDelta(t, tc.want.RefreshTokens, done["refresh_tokens"], 0)
			require.InDelta(t, tc.want.AuthSessions, done["auth_sessions"], 0)
			require.InDelta(t, tc.want.AuthTokens, done["auth_tokens"], 0)
			require.InDelta(t, tc.want.ActivityLogs, done["api_activity_logs"], 0)
			require.Contains(t, done, "duration_ms")
			if len(recs) > 1 {
				require.Equal(t, "refresh_tokens", recs[0]["table"])
				require.Equal(t, "ERROR", recs[0]["level"])
			}
		})
	}
}

func TestWorker_Run_U(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T, w *Worker, q *sqlc.MockQuerier)
	}{
		{name: "it stops on ctx done before the first run, deleting nothing", run: func(t *testing.T, w *Worker, _ *sqlc.MockQuerier) {
			w.firstRunDelay = time.Hour
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w.Run(ctx) // returns; the mock would fail on any delete
		}},
		{name: "it runs after the first delay and again every interval", run: func(t *testing.T, w *Worker, q *sqlc.MockQuerier) {
			w.firstRunDelay, w.interval = time.Millisecond, 5*time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			runs := make(chan struct{}, 10)
			q.EXPECT().DeleteExpiredRefreshTokens(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, int32) (int64, error) {
				runs <- struct{}{}
				return 0, nil
			}).MinTimes(2)
			q.EXPECT().DeleteOrphanSessions(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
			q.EXPECT().DeleteExpiredAuthTokens(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
			done := make(chan struct{})
			go func() { w.Run(ctx); close(done) }()
			<-runs
			<-runs
			cancel()
			<-done
		}},
		{name: "a done ctx stops a table between batches", run: func(t *testing.T, w *Worker, q *sqlc.MockQuerier) {
			ctx, cancel := context.WithCancel(context.Background())
			w.batchSize = 1
			q.EXPECT().DeleteExpiredRefreshTokens(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, int32) (int64, error) {
				cancel() // the batch was full, but the next one must not start
				return 1, nil
			}).Times(1)
			res := w.RunOnce(ctx)
			require.Equal(t, int64(1), res.RefreshTokens)
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
			defer slog.SetDefault(prev)
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tc.run(t, NewWorker(q, 0), q)
		})
	}
}

// recordingNotifier keeps the alerts it is given.
type recordingNotifier struct {
	mu  sync.Mutex
	got []notifier.Alert
}

func (r *recordingNotifier) Notify(_ context.Context, a notifier.Alert) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, a)
}

func TestWorker_Alerts_U(t *testing.T) {
	testCases := []struct {
		name string
		want []notifier.Alert
	}{
		{name: "a table's failure is an ops alert titled with the table",
			want: []notifier.Alert{{Level: notifier.LevelError, Title: "cleanup failed: auth_tokens", Text: sql.ErrConnDone.Error()}}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
			defer slog.SetDefault(prev)
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().DeleteExpiredRefreshTokens(gomock.Any(), gomock.Any()).Return(int64(0), nil)
			q.EXPECT().DeleteOrphanSessions(gomock.Any(), gomock.Any()).Return(int64(0), nil)
			q.EXPECT().DeleteExpiredAuthTokens(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			alerts := &recordingNotifier{}
			w := NewWorker(q, 0)
			w.SetNotifier(alerts)
			w.RunOnce(context.Background())
			require.Equal(t, tc.want, alerts.got)
		})
	}
}
