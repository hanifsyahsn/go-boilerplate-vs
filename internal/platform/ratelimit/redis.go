package ratelimit

import (
	"context"
	_ "embed" // the Lua scripts
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisTimeout bounds every Redis call; a timeout counts as Redis being down.
const RedisTimeout = 150 * time.Millisecond

var (
	//go:embed take.lua
	takeLua    string
	takeScript = redis.NewScript(takeLua)

	//go:embed return.lua
	returnLua    string
	returnScript = redis.NewScript(returnLua)

	resetScript = redis.NewScript(`return redis.call('DEL', KEYS[1])`)
)

// Redis is the shared Limiter: the same token bucket as Memory, kept in Redis
// (≥ 7) so every replica counts together. Each operation is one Lua script —
// atomic on the server, timed by Redis' own clock (TIME), so replicas with
// skewed clocks still agree. A bucket left alone for Per + 1 s expires, which
// is the same as full. Wrap it in WithFallback or FailOpen: on its own an
// error reaches the caller.
type Redis struct {
	client redis.Scripter // *redis.Client in prod
	rate   Rate
	prefix string // "rl:" + name + ":"
}

var _ Limiter = (*Redis)(nil)

// NewRedis returns a limiter of rate per key, its keys under "rl:<name>:".
// It panics on a rate without a positive Burst and Per, or whose token
// interval is under a microsecond: the limits are constants in main.
func NewRedis(client redis.Scripter, rate Rate, name string) *Redis {
	if rate.Burst <= 0 || rate.Per <= 0 || rate.interval().Microseconds() == 0 {
		panic(fmt.Sprintf("ratelimit.NewRedis(): Burst and Per must be > 0 with an interval of at least 1µs, got %+v", rate))
	}
	return &Redis{client: client, rate: rate, prefix: "rl:" + name + ":"}
}

// args are the scripts' ARGV: burst, interval in µs, ttl in ms.
func (r *Redis) args() []any {
	ttl := r.rate.Per + time.Second
	return []any{r.rate.Burst, r.rate.interval().Microseconds(), ttl.Milliseconds()}
}

func (r *Redis) Take(ctx context.Context, key string) (Decision, error) {
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()
	res, err := takeScript.Run(ctx, r.client, []string{r.prefix + key}, r.args()...).Int64Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("ratelimit.Redis.Take: %w", err)
	}
	if len(res) != 2 {
		return Decision{}, fmt.Errorf("ratelimit.Redis.Take: the script answered %d values, want 2", len(res))
	}
	if res[0] == 1 {
		return Decision{Allowed: true}, nil
	}
	return Decision{RetryAfter: time.Duration(res[1]) * time.Microsecond}, nil
}

func (r *Redis) Reset(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()
	if err := resetScript.Run(ctx, r.client, []string{r.prefix + key}).Err(); err != nil {
		return fmt.Errorf("ratelimit.Redis.Reset: %w", err)
	}
	return nil
}

func (r *Redis) Return(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()
	if err := returnScript.Run(ctx, r.client, []string{r.prefix + key}, r.args()...).Err(); err != nil {
		return fmt.Errorf("ratelimit.Redis.Return: %w", err)
	}
	return nil
}
