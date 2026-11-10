// Package ratelimit is an in-memory fixed-window counter. Counters live in the
// process, so they hold for one instance only (several instances would each
// count apart — move to a shared store such as Redis then). Keys are whatever the
// caller limits on: an IP, an email, "login:<identifier>".
package ratelimit

import (
	"sync"
	"time"
)

// Limiter allows at most limit hits per key in each window.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu        sync.Mutex
	counts    map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	start time.Time
	count int
}

// New returns a limiter of limit hits per window.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, counts: map[string]*bucket{}}
}

// Allow records a hit for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.current(key)
	b.count++
	return b.count <= l.limit
}

// Blocked reports whether key already used up its window, without a hit.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.current(key).count >= l.limit
}

// Reset forgets key (a successful login clears its failures).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.counts, key)
}

// current is key's bucket for this window; caller holds mu. Expired buckets
// are swept at most once per window, so the map doesn't grow forever.
func (l *Limiter) current(key string) *bucket {
	now := l.now()
	if now.Sub(l.lastSweep) >= l.window {
		for k, b := range l.counts {
			if now.Sub(b.start) >= l.window {
				delete(l.counts, k)
			}
		}
		l.lastSweep = now
	}

	b, ok := l.counts[key]
	if !ok || now.Sub(b.start) >= l.window {
		b = &bucket{start: now}
		l.counts[key] = b
	}
	return b
}
