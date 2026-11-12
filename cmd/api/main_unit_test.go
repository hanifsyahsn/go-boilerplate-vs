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

			stopBackground(stopJobs, &jobs, stopAlerts, &alertSender, 5*time.Second)

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
			stopBackground(stopJobs, &jobs, stopAlerts, &alertSender, 100*time.Millisecond)

			require.Less(t, time.Since(start), time.Second, "shutdown went on after the drain limit")
			require.Contains(t, buf.String(), "notifier: alerts not sent before shutdown")
		})
	}
}
