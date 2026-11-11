package ratelimit

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"
)

// storeDownLogEvery is how often a limiter logs that its store is down, at
// most: during an outage every request would log otherwise.
const storeDownLogEvery = 30 * time.Second

// storeDownLog throttles the "store unavailable" warning to one per
// storeDownLogEvery per limiter.
type storeDownLog struct {
	name string
	mode string
	last atomic.Int64 // unix nanos of the last warning
	now  func() time.Time
}

func (l *storeDownLog) warn(ctx context.Context, err error) {
	now := l.now().UnixNano()
	last := l.last.Load()
	if last != 0 && now-last < int64(storeDownLogEvery) {
		return
	}
	if !l.last.CompareAndSwap(last, now) {
		return // another request logged it just now
	}
	slog.WarnContext(ctx, "rate limit: store unavailable", "limiter", l.name, "mode", l.mode, "err", err)
}

// withFallback is WithFallback's Limiter.
type withFallback struct {
	primary, secondary Limiter
	log                *storeDownLog
}

// WithFallback uses primary, and secondary whenever primary returns an error
// (auth limits: never fail open). Reset / Return act on both, so the
// secondary's count stays close for the next outage; a secondary error is
// returned only when the primary failed too.
func WithFallback(primary, secondary Limiter, name string) Limiter {
	return &withFallback{primary: primary, secondary: secondary,
		log: &storeDownLog{name: name, mode: "fallback", now: time.Now}}
}

func (f *withFallback) Take(ctx context.Context, key string) (Decision, error) {
	d, err := f.primary.Take(ctx, key)
	if err == nil {
		return d, nil
	}
	f.log.warn(ctx, err)
	return f.secondary.Take(ctx, key)
}

func (f *withFallback) Reset(ctx context.Context, key string) error {
	return f.both(ctx, f.primary.Reset(ctx, key), f.secondary.Reset(ctx, key))
}

func (f *withFallback) Return(ctx context.Context, key string) error {
	return f.both(ctx, f.primary.Return(ctx, key), f.secondary.Return(ctx, key))
}

func (f *withFallback) both(ctx context.Context, primaryErr, secondaryErr error) error {
	if primaryErr == nil {
		return nil
	}
	f.log.warn(ctx, primaryErr)
	if secondaryErr == nil {
		return nil
	}
	return errors.Join(primaryErr, secondaryErr)
}

// failOpen is FailOpen's Limiter.
type failOpen struct {
	primary Limiter
	log     *storeDownLog
}

// FailOpen uses primary, and allows the request whenever primary returns an
// error (ordinary limits: a flood guard isn't worth an outage). Reset /
// Return swallow a primary error (logged).
func FailOpen(primary Limiter, name string) Limiter {
	return &failOpen{primary: primary, log: &storeDownLog{name: name, mode: "fail_open", now: time.Now}}
}

func (f *failOpen) Take(ctx context.Context, key string) (Decision, error) {
	d, err := f.primary.Take(ctx, key)
	if err != nil {
		f.log.warn(ctx, err)
		return Decision{Allowed: true}, nil
	}
	return d, nil
}

func (f *failOpen) Reset(ctx context.Context, key string) error {
	if err := f.primary.Reset(ctx, key); err != nil {
		f.log.warn(ctx, err)
	}
	return nil
}

func (f *failOpen) Return(ctx context.Context, key string) error {
	if err := f.primary.Return(ctx, key); err != nil {
		f.log.warn(ctx, err)
	}
	return nil
}
