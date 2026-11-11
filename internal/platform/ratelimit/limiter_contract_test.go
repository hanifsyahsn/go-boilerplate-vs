package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// runLimiterContract is the behaviour every Limiter shares, run against each
// implementation. newLimiter makes a fresh limiter (fresh keys too: tests
// use their own); advance moves that limiter's clock forward (a fake clock
// for Memory, a real sleep for Redis). The rate is 2 tokens per interval*2.
func runLimiterContract(t *testing.T, interval time.Duration, newLimiter func(Rate) Limiter, advance func(Limiter, time.Duration)) {
	t.Helper()
	rate := Rate{Burst: 2, Per: 2 * interval}
	ctx := context.Background()
	take := func(t *testing.T, l Limiter, key string) Decision {
		t.Helper()
		d, err := l.Take(ctx, key)
		require.NoError(t, err)
		return d
	}

	testCases := []struct {
		name string
		run  func(t *testing.T, l Limiter)
	}{
		{name: "burst is spent then refused", run: func(t *testing.T, l Limiter) {
			require.True(t, take(t, l, "k").Allowed)
			require.True(t, take(t, l, "k").Allowed)
			require.False(t, take(t, l, "k").Allowed)
		}},
		{name: "refused answer carries the wait for one token", run: func(t *testing.T, l Limiter) {
			take(t, l, "k")
			take(t, l, "k")
			d := take(t, l, "k")
			require.False(t, d.Allowed)
			require.Greater(t, d.RetryAfter, time.Duration(0))
			require.LessOrEqual(t, d.RetryAfter, interval)
		}},
		{name: "a token comes back after one interval", run: func(t *testing.T, l Limiter) {
			take(t, l, "k")
			take(t, l, "k")
			advance(l, interval+interval/5)
			require.True(t, take(t, l, "k").Allowed)
			require.False(t, take(t, l, "k").Allowed)
		}},
		{name: "reset refills at once", run: func(t *testing.T, l Limiter) {
			take(t, l, "k")
			take(t, l, "k")
			require.NoError(t, l.Reset(ctx, "k"))
			require.True(t, take(t, l, "k").Allowed)
			require.True(t, take(t, l, "k").Allowed)
		}},
		{name: "keys don't share a bucket", run: func(t *testing.T, l Limiter) {
			take(t, l, "a")
			take(t, l, "a")
			require.True(t, take(t, l, "b").Allowed)
		}},
		{name: "return gives one token back", run: func(t *testing.T, l Limiter) {
			take(t, l, "k")
			take(t, l, "k")
			require.NoError(t, l.Return(ctx, "k"))
			require.True(t, take(t, l, "k").Allowed)
			require.False(t, take(t, l, "k").Allowed, "one token, not the whole burst")
		}},
		{name: "return never goes past burst", run: func(t *testing.T, l Limiter) {
			take(t, l, "k")
			require.NoError(t, l.Return(ctx, "k"))
			require.NoError(t, l.Return(ctx, "k"))
			require.True(t, take(t, l, "k").Allowed)
			require.True(t, take(t, l, "k").Allowed)
			require.False(t, take(t, l, "k").Allowed)
		}},
		{name: "return on an unknown key is a no-op", run: func(t *testing.T, l Limiter) {
			require.NoError(t, l.Return(ctx, "never-seen"))
			require.True(t, take(t, l, "never-seen").Allowed)
			require.True(t, take(t, l, "never-seen").Allowed)
			require.False(t, take(t, l, "never-seen").Allowed)
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, newLimiter(rate))
		})
	}
}

func TestMemory_Contract_U(t *testing.T) {
	clocks := map[Limiter]*time.Time{}
	runLimiterContract(t, time.Minute, func(r Rate) Limiter {
		m := NewMemory(r)
		clock := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
		m.now = func() time.Time { return clock }
		clocks[m] = &clock
		return m
	}, func(l Limiter, d time.Duration) {
		*clocks[l] = clocks[l].Add(d)
	})
}
