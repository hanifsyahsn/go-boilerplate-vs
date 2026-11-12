package ratelimit

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

var (
	redisOnce   sync.Once
	redisClient *redis.Client
	redisErr    error
)

// testRedis starts one redis:7-alpine container for the package's tests.
func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	redisOnce.Do(func() {
		addr, _, err := testhelper.StartRedis(context.Background()) // Ryuk stops it with the test binary
		if err != nil {
			redisErr = err
			return
		}
		redisClient = redis.NewClient(&redis.Options{Addr: addr})
	})
	require.NoError(t, redisErr)
	return redisClient
}

func TestRedis_Contract_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	c := testRedis(t)
	runLimiterContract(t, 200*time.Millisecond, func(r Rate) Limiter {
		return NewRedis(c, r, uuid.NewString()) // a fresh name: fresh keys
	}, func(_ Limiter, d time.Duration) {
		time.Sleep(d)
	})
}

func TestRedis_Take_Parallel_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	c := testRedis(t)

	testCases := []struct {
		name        string
		parallel    int
		burst       int
		wantAllowed int64
	}{
		{name: "50 goroutines on a burst of 10: exactly 10 allowed", parallel: 50, burst: 10, wantAllowed: 10},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewRedis(c, Rate{Burst: tc.burst, Per: time.Hour}, uuid.NewString())
			var allowed atomic.Int64
			var wg sync.WaitGroup
			for range tc.parallel {
				wg.Add(1)
				go func() {
					defer wg.Done()
					d, err := l.Take(context.Background(), "k")
					if err == nil && d.Allowed {
						allowed.Add(1)
					}
				}()
			}
			wg.Wait()
			require.Equal(t, tc.wantAllowed, allowed.Load())
		})
	}
}

func TestRedis_KeyExpires_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	c := testRedis(t)

	testCases := []struct {
		name string
		rate Rate
	}{
		{name: "a bucket's key expires within Per + 1 s (left alone that long it is full again)", rate: Rate{Burst: 5, Per: time.Minute}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name := uuid.NewString()
			l := NewRedis(c, tc.rate, name)
			_, err := l.Take(context.Background(), "k")
			require.NoError(t, err)
			ttl, err := c.PTTL(context.Background(), "rl:"+name+":k").Result()
			require.NoError(t, err)
			require.Greater(t, ttl, time.Duration(0))
			require.LessOrEqual(t, ttl, tc.rate.Per+time.Second)
		})
	}
}

func TestRedis_Down_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name string
	}{
		{name: "an unreachable Redis is an error from every method, within the timeout"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
			defer func() { _ = c.Close() }()
			l := NewRedis(c, Rate{Burst: 1, Per: time.Minute}, "down")
			start := time.Now()
			_, err := l.Take(context.Background(), "k")
			require.ErrorContains(t, err, "ratelimit.Redis.Take")
			require.ErrorContains(t, l.Reset(context.Background(), "k"), "ratelimit.Redis.Reset")
			require.ErrorContains(t, l.Return(context.Background(), "k"), "ratelimit.Redis.Return")
			require.Less(t, time.Since(start), 3*time.Second)
		})
	}
}

func TestRedis_StoredStatePrecision_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	c := testRedis(t)

	testCases := []struct {
		name string
		take func(l *Redis) error
	}{
		{name: "after a take, the bucket's timestamp is stored as whole microseconds, every digit kept", take: func(l *Redis) error {
			_, err := l.Take(context.Background(), "k")
			return err
		}},
		{name: "after a return, the same", take: func(l *Redis) error {
			if _, err := l.Take(context.Background(), "k"); err != nil {
				return err
			}
			return l.Return(context.Background(), "k")
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name := uuid.NewString()
			l := NewRedis(c, Rate{Burst: 5, Per: time.Minute}, name)
			require.NoError(t, tc.take(l))

			ts, err := c.HGet(context.Background(), "rl:"+name+":k", "ts").Result()
			require.NoError(t, err)
			// Lua 5.1's tostring gave "1.7629434567891e+15": 14 digits, 100 µs.
			require.Regexp(t, `^\d{16}$`, ts, "a 16-digit µs timestamp, no exponent, no rounding")
			tm, err := c.Time(context.Background()).Result()
			require.NoError(t, err)
			stored, err := strconv.ParseInt(ts, 10, 64)
			require.NoError(t, err)
			require.InDelta(t, tm.UnixMicro(), stored, float64(time.Second.Microseconds()), "it is Redis' clock, in µs")
		})
	}
}
