package main

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
	"github.com/stretchr/testify/require"
)

// fakeSender behaves like notifier.Telegram.Run: it sends while its ctx is
// live, and on cancel sends what is queued, then returns.
type fakeSender struct {
	queue chan string
	mu    sync.Mutex
	sent  []string
}

func (f *fakeSender) Run(ctx context.Context) {
	for {
		select {
		case a := <-f.queue:
			f.send(a)
		case <-ctx.Done():
			for {
				select {
				case a := <-f.queue:
					f.send(a)
				default:
					return
				}
			}
		}
	}
}

func (f *fakeSender) send(a string) { f.mu.Lock(); defer f.mu.Unlock(); f.sent = append(f.sent, a) }

func (f *fakeSender) Sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

func TestStopBackground_U(t *testing.T) {
	testCases := []struct {
		name string
	}{
		{name: "an alert raised by a job while it shuts down is still sent"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jobsCtx, stopJobs := context.WithCancel(context.Background())
			alertCtx, stopAlerts := context.WithCancel(context.Background())
			var jobs, alertSender utility.Group
			sender := &fakeSender{queue: make(chan string, 10)}
			alertSender.Go(alertCtx, sender.Run)
			jobs.Go(jobsCtx, func(ctx context.Context) {
				<-ctx.Done()
				time.Sleep(50 * time.Millisecond) // the last flush takes a while, then fails
				sender.queue <- "activity log: write failed"
			})

			stopBackground(stopJobs, &jobs, stopAlerts, &alertSender, 5*time.Second, 5*time.Second)

			require.Equal(t, []string{"activity log: write failed"}, sender.Sent())
		})
	}
}

func TestStopBackground_DrainBounded_U(t *testing.T) {
	testCases := []struct {
		name string
	}{
		{name: "an alert sender that can't finish (Telegram down) doesn't hold shutdown past the drain limit"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			_, stopJobs := context.WithCancel(context.Background())
			alertCtx, stopAlerts := context.WithCancel(context.Background())
			var jobs, alertSender utility.Group
			release := make(chan struct{})
			defer close(release)
			// each send hangs, like a 5 s timeout against an unreachable API
			alertSender.Go(alertCtx, func(ctx context.Context) { <-ctx.Done(); <-release })

			start := time.Now()
			stopBackground(stopJobs, &jobs, stopAlerts, &alertSender, 5*time.Second, 100*time.Millisecond)

			// generous: a full make check starves goroutines for a second or more;
			// the regression this guards is waiting for ever
			require.Less(t, time.Since(start), 3*time.Second, "shutdown went on after the drain limit")
			require.Contains(t, buf.String(), "notifier: alerts not sent before shutdown")
		})
	}
}

func TestStopBackground_Deadline_U(t *testing.T) {
	testCases := []struct {
		name string
		// job runs in the jobs group; release ends whatever still blocks after the test.
		job        func(ctx context.Context, release <-chan struct{})
		sender     func(ctx context.Context, release <-chan struct{})
		total      time.Duration
		alertDrain time.Duration
		wantMax    time.Duration // stopBackground returns before this; a regression waits ≥ 5 s or for ever
		wantLogs   []string
		wantNoLogs []string
	}{
		{
			name:       "a job that ignores its cancel is given up at the deadline, warned, and shutdown goes on",
			job:        func(_ context.Context, release <-chan struct{}) { <-release },
			sender:     func(ctx context.Context, _ <-chan struct{}) { <-ctx.Done() },
			total:      100 * time.Millisecond,
			alertDrain: 5 * time.Second,
			wantMax:    3 * time.Second,
			wantLogs:   []string{"background: jobs not finished before shutdown", `"waited":"100ms"`},
			// the deadline is gone, yet a sender that finishes at once is seen finished (minWait)
			wantNoLogs: []string{"notifier: alerts not sent before shutdown"},
		},
		{
			name: "the alerts get only what the jobs left of the deadline, not their full drain limit",
			job: func(ctx context.Context, _ <-chan struct{}) {
				<-ctx.Done()
				time.Sleep(60 * time.Millisecond) // a slow last flush
			},
			sender:     func(ctx context.Context, release <-chan struct{}) { <-ctx.Done(); <-release }, // Telegram down
			total:      time.Second,                                                                    // the 60 ms flush fits even on a starved make check; full drain would be 5 s
			alertDrain: 5 * time.Second,
			wantMax:    3 * time.Second,
			wantLogs:   []string{"notifier: alerts not sent before shutdown"},
			wantNoLogs: []string{"background: jobs not finished before shutdown"},
		},
		{
			name:       "jobs that used up the whole deadline leave the alerts nothing: both are warned, no extra drain",
			job:        func(_ context.Context, release <-chan struct{}) { <-release },
			sender:     func(ctx context.Context, release <-chan struct{}) { <-ctx.Done(); <-release },
			total:      100 * time.Millisecond,
			alertDrain: 5 * time.Second,
			wantMax:    3 * time.Second,
			wantLogs:   []string{"background: jobs not finished before shutdown", "notifier: alerts not sent before shutdown", `"waited":"0s"`},
		},
		{
			name:       "jobs and alerts that finish in time: no warning",
			job:        func(ctx context.Context, _ <-chan struct{}) { <-ctx.Done() },
			sender:     func(ctx context.Context, _ <-chan struct{}) { <-ctx.Done() },
			total:      5 * time.Second,
			alertDrain: 5 * time.Second,
			wantMax:    3 * time.Second,
			wantNoLogs: []string{"not finished before shutdown", "not sent before shutdown"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			jobsCtx, stopJobs := context.WithCancel(context.Background())
			alertCtx, stopAlerts := context.WithCancel(context.Background())
			release := make(chan struct{})
			defer close(release)
			var jobs, alertSender utility.Group
			jobs.Go(jobsCtx, func(ctx context.Context) { tc.job(ctx, release) })
			alertSender.Go(alertCtx, func(ctx context.Context) { tc.sender(ctx, release) })

			start := time.Now()
			stopBackground(stopJobs, &jobs, stopAlerts, &alertSender, tc.total, tc.alertDrain)
			elapsed := time.Since(start)

			require.Less(t, elapsed, tc.wantMax, "shutdown waited past its deadline")
			for _, want := range tc.wantLogs {
				require.Contains(t, buf.String(), want)
			}
			for _, not := range tc.wantNoLogs {
				require.NotContains(t, buf.String(), not)
			}
		})
	}
}

func TestWaitFor_U(t *testing.T) {
	testCases := []struct {
		name    string
		running bool
		d       time.Duration
		want    bool
	}{
		{name: "a finished group is reported finished even with no time left", running: false, d: 0, want: true},
		{name: "a running group with no time left is waited for only minWait", running: true, d: -time.Second, want: false},
		{name: "a running group is waited for d (longer than minWait), then reported unfinished", running: true, d: 200 * time.Millisecond, want: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var g utility.Group
			release := make(chan struct{})
			defer close(release)
			if tc.running {
				g.Go(context.Background(), func(context.Context) { <-release })
			}
			start := time.Now()
			got := waitFor(&g, tc.d)
			elapsed := time.Since(start)
			require.Equal(t, tc.want, got)
			if tc.running {
				require.GreaterOrEqual(t, elapsed, max(tc.d, minWait), "waited less than d / minWait")
			}
			require.Less(t, elapsed, max(tc.d, minWait)+3*time.Second)
		})
	}
}
