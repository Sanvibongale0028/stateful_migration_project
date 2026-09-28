package limiter

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Result struct {
	Allowed   bool
	Remaining float64
}

type TokenBucket struct {
	Redis      *redis.Client
	Capacity   float64
	RefillRate float64
}

func NewTokenBucket(
	redisClient *redis.Client,
	capacity float64,
	refillRate float64,
) *TokenBucket {

	return &TokenBucket{
		Redis:      redisClient,
		Capacity:   capacity,
		RefillRate: refillRate,
	}
}

var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]

local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local data = redis.call(
    "HMGET",
    key,
    "tokens",
    "last_refill"
)

local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

if tokens == nil then
    tokens = capacity
    last_refill = now
end

local elapsed = now - last_refill

if elapsed < 0 then
    elapsed = 0
end

tokens = tokens + (elapsed * refill_rate)

if tokens > capacity then
    tokens = capacity
end

local allowed = 0

if tokens >= 1 then
    tokens = tokens - 1
    allowed = 1
end

redis.call(
    "HSET",
    key,
    "tokens",
    tokens,
    "last_refill",
    now
)

redis.call(
    "EXPIRE",
    key,
    3600
)

return {
    allowed,
    tostring(tokens)
}
`)

func (tb *TokenBucket) Allow(
	ctx context.Context,
	key string,
) (Result, error) {

	now := float64(
		time.Now().UnixNano(),
	) / 1e9

	result, err := tokenBucketScript.Run(
		ctx,
		tb.Redis,
		[]string{key},
		tb.Capacity,
		tb.RefillRate,
		now,
	).Result()

	if err != nil {
		return Result{}, err
	}

	values, ok := result.([]interface{})

	if !ok || len(values) != 2 {
		return Result{}, fmt.Errorf(
			"unexpected Redis response",
		)
	}

	allowedValue, ok := values[0].(int64)

	if !ok {
		return Result{}, fmt.Errorf(
			"invalid allowed value",
		)
	}

	remainingString, ok := values[1].(string)

	if !ok {
		return Result{}, fmt.Errorf(
			"invalid remaining value",
		)
	}

	remaining, err := strconv.ParseFloat(
		remainingString,
		64,
	)

	if err != nil {
		return Result{}, err
	}

	return Result{
		Allowed:   allowedValue == 1,
		Remaining: remaining,
	}, nil
}

func BuildKey(
	clientID string,
	resource string,
) string {

	return fmt.Sprintf(
		"rate_limit:%s:%s",
		clientID,
		resource,
	)
}
