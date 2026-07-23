package valkey

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"

	"alga/logger"
)

var incrExpireScript = NewLuaScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 then
	redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return c
`)

var loginIncrExpireScript = NewLuaScript(`
local lockKey = KEYS[2]
local attemptKey = KEYS[1]

local ttl = redis.call('TTL', lockKey)
if ttl > 0 then
	return {-1, ttl}
end

local c = redis.call('INCR', attemptKey)
if c == 1 then
	redis.call('EXPIRE', attemptKey, ARGV[1])
end

return {c, -1}
`)

// ValkeyRateLimiter implements api.RateLimiter using Valkey INCR + EXPIRE.
type ValkeyRateLimiter struct {
	client *Client
	burst  int
	window time.Duration
}

// NewRateLimiter creates a Valkey-backed rate limiter.
func NewRateLimiter(client *Client, burst int) *ValkeyRateLimiter {
	return &ValkeyRateLimiter{
		client: client,
		burst:  burst,
		window: time.Minute,
	}
}

// Allow checks if a request from the given IP is allowed.
func (rl *ValkeyRateLimiter) Allow(ip string) bool {
	if rl.client == nil {
		return true
	}
	ctx := context.Background()
	key := "alga:rate:" + ip

	count, err := incrExpireScript.Exec(ctx, rl.client.Client(), []string{key}, []string{strconv.FormatInt(int64(rl.window.Seconds()), 10)}).AsInt64()
	if err != nil {
		logger.Warn("rate limiter Valkey error, allowing request", "ip", ip, "error", err)
		return true
	}

	return count <= int64(rl.burst)
}

func (rl *ValkeyRateLimiter) Stop() {}

// ValkeyLoginRateLimiter implements api.LoginRateLimiter using Valkey.
type ValkeyLoginRateLimiter struct {
	client          *Client
	maxAttempts     int
	window          time.Duration
	lockoutDuration time.Duration
}

// NewLoginRateLimiter creates a Valkey-backed login rate limiter.
func NewLoginRateLimiter(client *Client, maxAttempts int, window, lockoutDuration time.Duration) *ValkeyLoginRateLimiter {
	return &ValkeyLoginRateLimiter{
		client:          client,
		maxAttempts:     maxAttempts,
		window:          window,
		lockoutDuration: lockoutDuration,
	}
}

// CheckLoginAllowed checks if login is allowed and increments the attempt counter.
func (lrl *ValkeyLoginRateLimiter) CheckLoginAllowed(ip string) (allowed bool, remaining int, lockedUntil *time.Time) {
	ctx := context.Background()

	lockKey := "alga:rate:login_lock:" + ip
	attemptKey := "alga:rate:login:" + ip

	result, err := loginIncrExpireScript.Exec(ctx, lrl.client.Client(), []string{attemptKey, lockKey}, []string{strconv.FormatInt(int64(lrl.window.Seconds()), 10)}).AsIntSlice()
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			return true, lrl.maxAttempts, nil
		}
		logger.Warn("login rate limiter Valkey error, denying login", "ip", ip, "error", err)
		return false, 0, nil
	}

	count := result[0]
	lockTTL := result[1]

	if lockTTL > 0 {
		lockEnd := time.Now().Add(time.Duration(lockTTL) * time.Second)
		return false, 0, &lockEnd
	}

	if int(count) >= lrl.maxAttempts {
		lockEnd := time.Now().Add(lrl.lockoutDuration)
		lrl.client.Do(ctx, lrl.client.Builder().Set().Key(lockKey).Value("1").PxMilliseconds(int64(lrl.lockoutDuration/time.Millisecond)).Build())
		lrl.client.Do(ctx, lrl.client.Builder().Del().Key(attemptKey).Build())
		return false, 0, &lockEnd
	}

	return true, lrl.maxAttempts - int(count), nil
}

// Reset clears the attempt counter for an IP.
func (lrl *ValkeyLoginRateLimiter) Reset(ip string) {
	ctx := context.Background()
	lrl.client.Do(ctx, lrl.client.Builder().Del().Key("alga:rate:login:"+ip).Build())
	lrl.client.Do(ctx, lrl.client.Builder().Del().Key("alga:rate:login_lock:"+ip).Build())
}

func (lrl *ValkeyLoginRateLimiter) Stop() {}
