package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"
)

// DefaultMaxKeys is how many buckets a Memory keeps at most (a few MB).
const DefaultMaxKeys = 100_000

// Memory is the in-process Limiter. Only buckets that aren't full are kept: a
// full bucket is the same as no bucket, so one is dropped once it has refilled.
// The map is swept once per Rate.Per, and when it's full at most once per token
// interval — a full map of buckets that can't be dropped yet would otherwise be
// walked on every request, which a flood could use to burn CPU.
//
// When maxKeys buckets are all still refilling, Take refuses a key it has no
// bucket for, rather than forget one that is being limited (forgetting would
// let someone flood the map with fresh keys to wipe a victim's count). That
// takes maxKeys clients each spending tokens within one Rate.Per.
type Memory struct {
	rate    Rate
	maxKeys int
	now     func() time.Time

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
	// sweeps counts the sweeps run (for tests).
	sweeps int
}

type bucket struct {
	tokens float64
	// at is when tokens was last brought up to date.
	at time.Time
}

var _ Limiter = (*Memory)(nil)

// NewMemory returns an in-process limiter of rate per key. It panics on a rate
// without a positive Burst and Per, or one whose token interval (Per / Burst)
// rounds to zero: refill would divide by it. The limits are constants in main.
func NewMemory(rate Rate) *Memory {
	if rate.Burst <= 0 || rate.Per <= 0 || rate.interval() <= 0 {
		panic(fmt.Sprintf("ratelimit.NewMemory(): Burst and Per must be > 0 with a token interval above zero, got %+v", rate))
	}
	now := time.Now
	return &Memory{rate: rate, maxKeys: DefaultMaxKeys, now: now, buckets: map[string]*bucket{}, lastSweep: now()}
}

func (m *Memory) Take(ctx context.Context, key string) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	m.sweep(now)
	b, ok := m.buckets[key]
	if !ok {
		if len(m.buckets) >= m.maxKeys {
			slog.WarnContext(ctx, "rate limit: bucket map full, new key refused", "max_keys", m.maxKeys)
			return Decision{RetryAfter: m.rate.interval()}, nil
		}
		b = &bucket{tokens: float64(m.rate.Burst), at: now}
		m.buckets[key] = b
	}
	m.refill(b, now)
	if b.tokens < 1 {
		return Decision{RetryAfter: m.wait(b)}, nil
	}
	b.tokens--
	return Decision{Allowed: true}, nil
}

func (m *Memory) Reset(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.buckets, key)
	return nil
}

func (m *Memory) Return(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.buckets[key]
	if !ok {
		return nil
	}
	m.refill(b, m.now())
	b.tokens = math.Min(float64(m.rate.Burst), b.tokens+1)
	if b.tokens >= float64(m.rate.Burst) {
		delete(m.buckets, key) // full is the same as no bucket
	}
	return nil
}

// refill brings b's tokens up to now; caller holds mu.
func (m *Memory) refill(b *bucket, now time.Time) {
	if elapsed := now.Sub(b.at); elapsed > 0 {
		b.tokens = math.Min(float64(m.rate.Burst), b.tokens+float64(elapsed)/float64(m.rate.interval()))
		b.at = now
	}
}

// wait is how long b needs for its next whole token, to the nanosecond (the
// float tokens are rounded, not ceiled, so float noise doesn't add 1ns), and
// never less than 1ns: a refusal always has a wait, so its 429 always carries
// Retry-After (a bucket a hair under one token would otherwise round to 0);
// caller holds mu.
func (m *Memory) wait(b *bucket) time.Duration {
	return max(time.Nanosecond, time.Duration(math.Round((1-b.tokens)*float64(m.rate.interval()))))
}

// sweep drops the buckets that are full again: once per Rate.Per, or, while
// the map is full, once per token interval — so a full map costs one walk per
// interval, not one per request. A new key may wait up to one interval for a
// bucket that refilled in between; caller holds mu.
func (m *Memory) sweep(now time.Time) {
	since := now.Sub(m.lastSweep)
	full := len(m.buckets) >= m.maxKeys
	if since < m.rate.Per && (!full || since < m.rate.interval()) {
		return
	}
	m.sweeps++
	for k, b := range m.buckets {
		m.refill(b, now)
		if b.tokens >= float64(m.rate.Burst) {
			delete(m.buckets, k)
		}
	}
	m.lastSweep = now
}
