package middleware

import (
	"context"
	"time"

	"github.com/orislabsdev/gocore/config"
	"github.com/redis/go-redis/v9"
)

// redisLimiterBackend uses Redis to track token-bucket limits.
type redisLimiterBackend struct {
	client *redis.Client
	cfg    config.RateLimitConfig
}

// newRedisLimiterBackend creates a Redis client using the provided configuration.
func newRedisLimiterBackend(cfg config.RateLimitConfig) *redisLimiterBackend {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Address,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.ReadTimeout,
	})

	return &redisLimiterBackend{
		client: client,
		cfg:    cfg,
	}
}

// redisTokenBucketScript tracks the token bucket algorithm atomicity in Redis.
// KEYS[1] : rate limit key (e.g., ratelimit:{ip})
// ARGV[1] : burst limit (max tokens)
// ARGV[2] : requests per second (refill rate)
// ARGV[3] : current timestamp in seconds
var redisTokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local amount = 1

local res = redis.call('HMGET', key, 'tokens', 'last_refresh')
local tokens = tonumber(res[1])
local last_refresh = tonumber(res[2])

if tokens == nil then
    tokens = limit
    last_refresh = now
end

local refill = (now - last_refresh) * rate
if refill > 0 then
    tokens = tokens + refill
    if tokens > limit then
        tokens = limit
    end
    last_refresh = now
end

local allowed = 0
local retry_after = 0

if tokens >= amount then
    allowed = 1
    tokens = tokens - amount
else
    allowed = 0
    retry_after = math.ceil((amount - tokens) / rate)
end

redis.call('HMSET', key, 'tokens', tokens, 'last_refresh', last_refresh)

local full_time = math.ceil((limit - tokens) / rate)
if full_time < 0 then
    full_time = 0
end
redis.call('EXPIRE', key, full_time + 2)

return {allowed, retry_after}
`)

func (b *redisLimiterBackend) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	if b.cfg.RequestsPerSecond <= 0 {
		return true, 0, nil
	}

	redisKey := "ratelimit:" + key
	// We use Unix time as float64 to track fractional seconds.
	now := float64(time.Now().UnixNano()) / 1e9

	// Script returns {allowed (1 or 0), retry_after (integer seconds)}
	res, err := redisTokenBucketScript.Run(ctx, b.client, []string{redisKey}, float64(b.cfg.Burst), b.cfg.RequestsPerSecond, now).Result()
	if err != nil {
		return false, 0, err
	}

	vals, ok := res.([]interface{})
	if !ok || len(vals) < 2 {
		return false, 0, nil
	}

	allowedInt, _ := vals[0].(int64)
	retryAfterInt, _ := vals[1].(int64)

	allowed := (allowedInt == 1)
	retryAfter := time.Duration(retryAfterInt) * time.Second

	return allowed, retryAfter, nil
}
