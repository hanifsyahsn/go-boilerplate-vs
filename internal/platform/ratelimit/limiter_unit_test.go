package ratelimit

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/stretchr/testify/require"
)

// take is m.Take for a test: it never fails.
func take(t *testing.T, m *Memory, key string) Decision {
	t.Helper()
	d, err := m.Take(context.Background(), key)
	require.NoError(t, err)
	return d
}

func TestMemory_U(t *testing.T) {
	allowed := Decision{Allowed: true}

	testCases := []struct {
		name string
		// run drives a 2-per-minute limiter (a token every 30s) whose clock
		// is *now and whose map holds at most 3 buckets.
		run func(t *testing.T, m *Memory, now *time.Time)
	}{
		{
			name: "allows the burst, then refuses with the wait for the next token",
			run: func(t *testing.T, m *Memory, _ *time.Time) {
				require.Equal(t, allowed, take(t, m, "a"))
				require.Equal(t, allowed, take(t, m, "a"))
				require.Equal(t, Decision{RetryAfter: 30 * time.Second}, take(t, m, "a"))
			},
		},
		{
			name: "keys have their own buckets",
			run: func(t *testing.T, m *Memory, _ *time.Time) {
				take(t, m, "a")
				take(t, m, "a")
				require.Equal(t, allowed, take(t, m, "b"))
			},
		},
		{
			name: "one token comes back per interval, not the whole burst at once",
			run: func(t *testing.T, m *Memory, now *time.Time) {
				take(t, m, "a")
				take(t, m, "a")
				*now = now.Add(30 * time.Second)
				require.Equal(t, allowed, take(t, m, "a"))
				require.Equal(t, Decision{RetryAfter: 30 * time.Second}, take(t, m, "a"))
			},
		},
		{
			name: "the wait counts down a partly refilled token",
			run: func(t *testing.T, m *Memory, now *time.Time) {
				take(t, m, "a")
				take(t, m, "a")
				*now = now.Add(20 * time.Second)
				require.Equal(t, Decision{RetryAfter: 10 * time.Second}, take(t, m, "a"))
			},
		},
		{
			name: "a long pause refills up to the burst, never beyond",
			run: func(t *testing.T, m *Memory, now *time.Time) {
				take(t, m, "a")
				*now = now.Add(time.Hour)
				require.Equal(t, allowed, take(t, m, "a"))
				require.Equal(t, allowed, take(t, m, "a"))
				require.False(t, take(t, m, "a").Allowed)
			},
		},
		{
			name: "Return gives one token back, not the whole burst",
			run: func(t *testing.T, m *Memory, _ *time.Time) {
				take(t, m, "a")
				take(t, m, "a")
				require.NoError(t, m.Return(context.Background(), "a"))
				require.Equal(t, allowed, take(t, m, "a"))
				require.Equal(t, Decision{RetryAfter: 30 * time.Second}, take(t, m, "a"))
			},
		},
		{
			name: "Return adds to what refilled meanwhile",
			run: func(t *testing.T, m *Memory, now *time.Time) {
				take(t, m, "a")
				take(t, m, "a")
				*now = now.Add(20 * time.Second) // 2/3 of a token back
				require.NoError(t, m.Return(context.Background(), "a"))
				require.Equal(t, allowed, take(t, m, "a"))
				require.Equal(t, Decision{RetryAfter: 10 * time.Second}, take(t, m, "a"))
			},
		},
		{
			name: "Return never goes past the burst, and a bucket it fills is dropped",
			run: func(t *testing.T, m *Memory, _ *time.Time) {
				take(t, m, "a")
				require.NoError(t, m.Return(context.Background(), "a"))
				require.NoError(t, m.Return(context.Background(), "a"))
				require.Empty(t, m.buckets)
				require.Equal(t, allowed, take(t, m, "a"))
				require.Equal(t, allowed, take(t, m, "a"))
				require.False(t, take(t, m, "a").Allowed)
			},
		},
		{
			name: "Return on a key with no bucket is a no-op",
			run: func(t *testing.T, m *Memory, _ *time.Time) {
				require.NoError(t, m.Return(context.Background(), "z"))
				require.Empty(t, m.buckets)
				require.Equal(t, allowed, take(t, m, "z"))
				require.Equal(t, allowed, take(t, m, "z"))
				require.False(t, take(t, m, "z").Allowed)
			},
		},
		{
			name: "Reset refills the key's bucket",
			run: func(t *testing.T, m *Memory, _ *time.Time) {
				take(t, m, "a")
				take(t, m, "a")
				require.NoError(t, m.Reset(context.Background(), "a"))
				require.Equal(t, allowed, take(t, m, "a"))
				require.Equal(t, allowed, take(t, m, "a"))
			},
		},
		{
			name: "buckets that refilled are dropped once a refill period has passed",
			run: func(t *testing.T, m *Memory, now *time.Time) {
				take(t, m, "a")
				take(t, m, "b")
				take(t, m, "b")
				*now = now.Add(time.Minute)
				take(t, m, "c")
				require.Len(t, m.buckets, 1, "only c, just used, is kept")
			},
		},
		{
			name: "a full map makes room by dropping refilled buckets",
			run: func(t *testing.T, m *Memory, now *time.Time) {
				take(t, m, "a")
				take(t, m, "b")
				take(t, m, "c")
				*now = now.Add(30 * time.Second) // one token back: a, b, c are full again
				require.Equal(t, allowed, take(t, m, "d"))
				require.Len(t, m.buckets, 1)
			},
		},
		{
			name: "a full map of buckets still refilling refuses a new key and keeps the limited ones",
			run: func(t *testing.T, m *Memory, _ *time.Time) {
				var buf bytes.Buffer
				l, err := logging.New(&buf, logging.FormatJSON, "debug")
				require.NoError(t, err)
				prev := slog.Default()
				slog.SetDefault(l)
				defer slog.SetDefault(prev)

				for _, k := range []string{"victim", "victim", "b", "c"} {
					take(t, m, k)
				}
				require.Equal(t, Decision{RetryAfter: 30 * time.Second}, take(t, m, "fresh"))
				require.False(t, take(t, m, "victim").Allowed, "the victim's count wasn't wiped")
				require.Contains(t, buf.String(), `"msg":"rate limit: bucket map full, new key refused"`)
				require.Contains(t, buf.String(), `"max_keys":3`)
			},
		},
		{
			name: "a full map is walked once per token interval, not on every request",
			run: func(t *testing.T, m *Memory, now *time.Time) {
				for _, k := range []string{"a", "b", "c"} {
					take(t, m, k)
				}
				take(t, m, "a")
				take(t, m, "b")
				take(t, m, "c") // all three empty: nothing to drop for 30s
				for range 50 {
					take(t, m, "fresh")
				}
				require.Equal(t, 0, m.sweeps, "within the interval the full map isn't walked again")
				*now = now.Add(30 * time.Second)
				take(t, m, "fresh")
				take(t, m, "fresh")
				require.Equal(t, 1, m.sweeps, "one walk once the interval has passed")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
			m := NewMemory(Rate{Burst: 2, Per: time.Minute})
			m.now = func() time.Time { return now }
			m.lastSweep = now
			m.maxKeys = 3
			tc.run(t, m, &now)
		})
	}
}

// TestMemory_Take_Parallel_U pins that Take is atomic: of many parallel takes
// on one key, exactly the burst are allowed.
func TestMemory_Take_Parallel_U(t *testing.T) {
	testCases := []struct {
		name        string
		burst       int
		parallel    int
		wantAllowed int
	}{
		{name: "more takers than tokens: exactly the burst get through", burst: 5, parallel: 200, wantAllowed: 5},
		{name: "fewer takers than tokens: all get through", burst: 5, parallel: 3, wantAllowed: 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMemory(Rate{Burst: tc.burst, Per: time.Hour})
			var wg sync.WaitGroup
			var allowed atomic.Int32
			start := make(chan struct{})
			for range tc.parallel {
				wg.Go(func() {
					<-start
					if d, _ := m.Take(context.Background(), "k"); d.Allowed {
						allowed.Add(1)
					}
				})
			}
			close(start)
			wg.Wait()
			require.Equal(t, int32(tc.wantAllowed), allowed.Load())
		})
	}
}

func TestNewMemory_U(t *testing.T) {
	testCases := []struct {
		name      string
		rate      Rate
		wantPanic bool
	}{
		{name: "a positive burst and period make a limiter", rate: Rate{Burst: 5, Per: time.Minute}},
		{name: "a zero burst is refused", rate: Rate{Burst: 0, Per: time.Minute}, wantPanic: true},
		{name: "a zero period is refused", rate: Rate{Burst: 5}, wantPanic: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic {
				require.Panics(t, func() { NewMemory(tc.rate) })
				return
			}
			m := NewMemory(tc.rate)
			require.Equal(t, DefaultMaxKeys, m.maxKeys)
		})
	}
}

func TestIPKey_U(t *testing.T) {
	testCases := []struct {
		name string
		ip   string
		want string
	}{
		{name: "an IPv4 address is its own key", ip: "203.0.113.7", want: "203.0.113.7"},
		{name: "an IPv4-mapped IPv6 address keys as the IPv4 one", ip: "::ffff:203.0.113.7", want: "203.0.113.7"},
		{name: "an IPv6 address keys as its /64", ip: "2001:db8:1:2:aaaa:bbbb:cccc:dddd", want: "2001:db8:1:2::/64"},
		{name: "two addresses in one /64 share a key", ip: "2001:db8:1:2::1", want: "2001:db8:1:2::/64"},
		{name: "something that isn't an IP is kept as it is", ip: "not-an-ip", want: "not-an-ip"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, IPKey(tc.ip))
		})
	}
}
