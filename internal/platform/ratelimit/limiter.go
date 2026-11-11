// Package ratelimit is the rate limiter behind every limit of the API: a token
// bucket per key, behind the Limiter interface (decision 0003). Keys are
// whatever the caller limits on: an IP (IPKey), "login:<email>".
//
// Memory (NewMemory) keeps the buckets in the process, so it holds for one
// instance only: several replicas would each count apart.
package ratelimit

import (
	"context"
	"net/netip"
	"time"
)

// Rate is a bucket's size and refill: Burst tokens, refilled evenly over Per
// (Burst / Per tokens a second). "5 failed logins / 15 min" is {5, 15 min}:
// five at once, then one more every three minutes.
type Rate struct {
	Burst int
	Per   time.Duration
}

// interval is how long one token takes to come back.
func (r Rate) interval() time.Duration {
	return r.Per / time.Duration(r.Burst)
}

// Decision is a limiter's answer for one key.
type Decision struct {
	// Allowed: the key had a token, and Take took it.
	Allowed bool
	// RetryAfter is how long until the key has a token again; 0 when Allowed.
	RetryAfter time.Duration
}

// Limiter is a token bucket per key. Every method can fail (a shared store
// that's down); Memory never does.
type Limiter interface {
	// Take takes a token from key's bucket when it has one. It's atomic: N
	// parallel Takes on a key with k tokens allow exactly min(N, k). There is
	// no "peek, then take later" on purpose — that gap is a race.
	Take(ctx context.Context, key string) (Decision, error)
	// Reset refills key's bucket (a successful login clears its failures).
	Reset(ctx context.Context, key string) error
	// Return gives one token back to key's bucket, never past Burst: the
	// request that took it turned out not to count (a login with the right
	// password, on a budget other logins share). Unlike Reset it leaves the
	// rest of the count alone. A key with no bucket (full) is a no-op.
	Return(ctx context.Context, key string) error
}

// IPKey is the key of a client IP: the address itself for IPv4, its /64
// network for IPv6 — one subscriber usually gets a whole /64, so keying on the
// full address would hand a client 2^64 fresh buckets. A string that isn't an
// IP is returned as it is.
func IPKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String()
	}
	prefix, _ := addr.Prefix(64) // a valid IPv6 address always has a /64
	return prefix.String()
}
