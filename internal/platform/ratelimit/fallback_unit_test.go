package ratelimit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/stretchr/testify/require"
)

// stubLimiter answers with a fixed decision / errors and counts calls.
type stubLimiter struct {
	decision                  Decision
	takeErr, resetErr, retErr error
	takes, resets, returns    int
}

func (s *stubLimiter) Take(context.Context, string) (Decision, error) {
	s.takes++
	return s.decision, s.takeErr
}
func (s *stubLimiter) Reset(context.Context, string) error  { s.resets++; return s.resetErr }
func (s *stubLimiter) Return(context.Context, string) error { s.returns++; return s.retErr }

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	l, err := logging.New(&buf, logging.FormatJSON, "debug")
	require.NoError(t, err)
	prev := slog.Default()
	slog.SetDefault(l)
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func warnings(buf *bytes.Buffer) int {
	return strings.Count(buf.String(), `"msg":"rate limit: store unavailable"`)
}

var errDown = errors.New("redis down")

func TestWithFallback_U(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T, logs *bytes.Buffer)
	}{
		{name: "a healthy primary decides; the secondary isn't asked", run: func(t *testing.T, logs *bytes.Buffer) {
			p, s := &stubLimiter{decision: Decision{Allowed: false, RetryAfter: time.Second}}, &stubLimiter{decision: Decision{Allowed: true}}
			d, err := WithFallback(p, s, "auth").Take(context.Background(), "k")
			require.NoError(t, err)
			require.Equal(t, Decision{RetryAfter: time.Second}, d)
			require.Zero(t, s.takes)
			require.Zero(t, warnings(logs))
		}},
		{name: "a failing primary: the secondary decides (never fail open), warned once in a burst of calls", run: func(t *testing.T, logs *bytes.Buffer) {
			p, s := &stubLimiter{takeErr: errDown}, &stubLimiter{decision: Decision{Allowed: false}}
			l := WithFallback(p, s, "auth")
			for range 5 {
				d, err := l.Take(context.Background(), "k")
				require.NoError(t, err)
				require.False(t, d.Allowed, "the secondary's refusal stands")
			}
			require.Equal(t, 5, s.takes)
			require.Equal(t, 1, warnings(logs))
			require.Contains(t, logs.String(), `"limiter":"auth"`)
			require.Contains(t, logs.String(), `"mode":"fallback"`)
		}},
		{name: "the warning comes again after 30 s", run: func(t *testing.T, logs *bytes.Buffer) {
			clock := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
			l := WithFallback(&stubLimiter{takeErr: errDown}, &stubLimiter{}, "auth").(*withFallback)
			l.log.now = func() time.Time { return clock }
			_, _ = l.Take(context.Background(), "k")
			clock = clock.Add(29 * time.Second)
			_, _ = l.Take(context.Background(), "k")
			clock = clock.Add(2 * time.Second)
			_, _ = l.Take(context.Background(), "k")
			require.Equal(t, 2, warnings(logs))
		}},
		{name: "reset and return act on both", run: func(t *testing.T, _ *bytes.Buffer) {
			p, s := &stubLimiter{}, &stubLimiter{}
			l := WithFallback(p, s, "auth")
			require.NoError(t, l.Reset(context.Background(), "k"))
			require.NoError(t, l.Return(context.Background(), "k"))
			require.Equal(t, []int{1, 1, 1, 1}, []int{p.resets, s.resets, p.returns, s.returns})
		}},
		{name: "a secondary error alone isn't returned", run: func(t *testing.T, _ *bytes.Buffer) {
			l := WithFallback(&stubLimiter{}, &stubLimiter{resetErr: errDown, retErr: errDown}, "auth")
			require.NoError(t, l.Reset(context.Background(), "k"))
			require.NoError(t, l.Return(context.Background(), "k"))
		}},
		{name: "a primary error alone is logged, not returned", run: func(t *testing.T, logs *bytes.Buffer) {
			l := WithFallback(&stubLimiter{resetErr: errDown}, &stubLimiter{}, "auth")
			require.NoError(t, l.Reset(context.Background(), "k"))
			require.Equal(t, 1, warnings(logs))
		}},
		{name: "both failing: both errors returned", run: func(t *testing.T, _ *bytes.Buffer) {
			memErr := errors.New("memory refused")
			l := WithFallback(&stubLimiter{retErr: errDown}, &stubLimiter{retErr: memErr}, "auth")
			err := l.Return(context.Background(), "k")
			require.ErrorIs(t, err, errDown)
			require.ErrorIs(t, err, memErr)
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, captureLogs(t))
		})
	}
}

func TestFailOpen_U(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T, logs *bytes.Buffer)
	}{
		{name: "a healthy primary decides", run: func(t *testing.T, logs *bytes.Buffer) {
			d, err := FailOpen(&stubLimiter{decision: Decision{RetryAfter: time.Second}}, "public").Take(context.Background(), "k")
			require.NoError(t, err)
			require.False(t, d.Allowed)
			require.Zero(t, warnings(logs))
		}},
		{name: "an error is allowed, warned once in a burst", run: func(t *testing.T, logs *bytes.Buffer) {
			l := FailOpen(&stubLimiter{takeErr: errDown}, "public")
			for range 5 {
				d, err := l.Take(context.Background(), "k")
				require.NoError(t, err)
				require.True(t, d.Allowed)
			}
			require.Equal(t, 1, warnings(logs))
			require.Contains(t, logs.String(), `"mode":"fail_open"`)
		}},
		{name: "reset and return swallow a primary error (logged)", run: func(t *testing.T, logs *bytes.Buffer) {
			l := FailOpen(&stubLimiter{resetErr: errDown, retErr: errDown}, "public")
			require.NoError(t, l.Reset(context.Background(), "k"))
			require.NoError(t, l.Return(context.Background(), "k"))
			require.Equal(t, 1, warnings(logs))
		}},
		{name: "reset and return pass through when healthy", run: func(t *testing.T, logs *bytes.Buffer) {
			p := &stubLimiter{}
			l := FailOpen(p, "public")
			require.NoError(t, l.Reset(context.Background(), "k"))
			require.NoError(t, l.Return(context.Background(), "k"))
			require.Equal(t, []int{1, 1}, []int{p.resets, p.returns})
			require.Zero(t, warnings(logs))
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, captureLogs(t))
		})
	}
}

func TestNewRedis_U(t *testing.T) {
	testCases := []struct {
		name      string
		rate      Rate
		wantPanic bool
	}{
		{name: "a positive rate makes a limiter keyed under rl:<name>:", rate: Rate{Burst: 5, Per: time.Minute}},
		{name: "a zero burst is refused", rate: Rate{Per: time.Minute}, wantPanic: true},
		{name: "a zero period is refused", rate: Rate{Burst: 5}, wantPanic: true},
		{name: "an interval under a microsecond is refused", rate: Rate{Burst: 1000, Per: time.Microsecond}, wantPanic: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic {
				require.Panics(t, func() { NewRedis(nil, tc.rate, "x") })
				return
			}
			require.Equal(t, "rl:auth:", NewRedis(nil, tc.rate, "auth").prefix)
		})
	}
}
