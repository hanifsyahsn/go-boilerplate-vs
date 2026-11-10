package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLimiter_U(t *testing.T) {
	testCases := []struct {
		name string
		// run drives a 2-per-minute limiter whose clock is *now.
		run func(t *testing.T, l *Limiter, now *time.Time)
	}{
		{
			name: "allows up to the limit, then refuses",
			run: func(t *testing.T, l *Limiter, _ *time.Time) {
				require.True(t, l.Allow("a"))
				require.True(t, l.Allow("a"))
				require.False(t, l.Allow("a"))
			},
		},
		{
			name: "keys are counted apart",
			run: func(t *testing.T, l *Limiter, _ *time.Time) {
				require.True(t, l.Allow("a"))
				require.True(t, l.Allow("a"))
				require.True(t, l.Allow("b"))
			},
		},
		{
			name: "a new window starts over",
			run: func(t *testing.T, l *Limiter, now *time.Time) {
				l.Allow("a")
				l.Allow("a")
				*now = now.Add(time.Minute)
				require.True(t, l.Allow("a"))
			},
		},
		{
			name: "Blocked reports a used-up key without counting a hit",
			run: func(t *testing.T, l *Limiter, _ *time.Time) {
				require.False(t, l.Blocked("a"))
				l.Allow("a")
				require.False(t, l.Blocked("a"))
				l.Allow("a")
				require.True(t, l.Blocked("a"))
				require.True(t, l.Blocked("a"))
			},
		},
		{
			name: "Reset forgets the key",
			run: func(t *testing.T, l *Limiter, _ *time.Time) {
				l.Allow("a")
				l.Allow("a")
				l.Reset("a")
				require.False(t, l.Blocked("a"))
				require.True(t, l.Allow("a"))
			},
		},
		{
			name: "expired buckets are swept",
			run: func(t *testing.T, l *Limiter, now *time.Time) {
				l.Allow("a")
				l.Allow("b")
				*now = now.Add(2 * time.Minute)
				l.Allow("c")
				require.Len(t, l.counts, 1)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
			l := New(2, time.Minute)
			l.now = func() time.Time { return now }
			l.lastSweep = now
			tc.run(t, l, &now)
		})
	}
}
