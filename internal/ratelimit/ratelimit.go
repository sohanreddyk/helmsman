package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter implements a token-bucket rate limiter backed by Redis.
// Each API key gets its own bucket. The bucket refills at `rate` tokens/sec
// up to a maximum of `burst`. The algorithm is implemented as a Lua script
// executed atomically on Redis to avoid race conditions across gateway replicas.
type Limiter struct {
	rdb   *redis.Client
	rate  int // tokens per second
	burst int // max bucket size
}

func New(rdb *redis.Client, rate, burst int) *Limiter {
	return &Limiter{rdb: rdb, rate: rate, burst: burst}
}

// luaTokenBucket is a Lua script that atomically implements the token bucket.
// It stores two values per key: the token count and the last refill timestamp.
// Returns 1 if the request is allowed, 0 if rate-limited.
var luaTokenBucket = redis.NewScript(`
local key       = KEYS[1]
local rate      = tonumber(ARGV[1])
local burst     = tonumber(ARGV[2])
local now       = tonumber(ARGV[3])
local ttl       = tonumber(ARGV[4])

local tokens_key = key .. ":tokens"
local ts_key     = key .. ":ts"

local tokens = tonumber(redis.call("GET", tokens_key))
local last_ts = tonumber(redis.call("GET", ts_key))

if tokens == nil then
    tokens  = burst
    last_ts = now
end

-- Refill: add tokens proportional to time elapsed
local elapsed = math.max(0, now - last_ts)
local refill  = math.floor(elapsed * rate)
tokens = math.min(burst, tokens + refill)

local allowed = 0
if tokens >= 1 then
    tokens  = tokens - 1
    allowed = 1
end

redis.call("SET", tokens_key, tokens, "EX", ttl)
redis.call("SET", ts_key,     now,    "EX", ttl)

return allowed
`)

// Allow returns true if the given key is within its rate limit.
func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().Unix()
	ttl := l.burst/l.rate + 10 // bucket expires after it would fully refill + buffer

	result, err := luaTokenBucket.Run(ctx, l.rdb,
		[]string{fmt.Sprintf("rl:%s", key)},
		l.rate, l.burst, now, ttl,
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}
