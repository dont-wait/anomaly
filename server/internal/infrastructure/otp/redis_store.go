package otp

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

// consumeScript: atomic compare-and-delete.
// KEYS[1] = otp key, ARGV[1] = expected code.
// Returns: 2 = consumed, 1 = mismatch, 0 = key missing.
var consumeScript = redis.NewScript(`
local stored = redis.call('GET', KEYS[1])
if not stored then
	return 0
end
if stored ~= ARGV[1] then
	return 1
end
redis.call('DEL', KEYS[1])
return 2
`)

// delIfMatchScript: conditional delete.
// KEYS[1] = otp key, ARGV[1] = expected code.
// Always returns 1 (best-effort; no error on mismatch/missing).
var delIfMatchScript = redis.NewScript(`
local stored = redis.call('GET', KEYS[1])
if stored == ARGV[1] then
	redis.call('DEL', KEYS[1])
end
return 1
`)

// incrAttemptsScript: atomic increment with TTL repair.
// KEYS[1] = attempt key, ARGV[1] = ttl in milliseconds.
// Returns the new counter value.
var incrAttemptsScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if redis.call('PTTL', KEYS[1]) == -1 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

type RedisStore struct {
	rdb *redis.Client
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

func (s *RedisStore) Set(ctx context.Context, email, code string, ttl time.Duration) error {
	return s.rdb.Set(ctx, otpdomain.Key(email), code, ttl).Err()
}

func (s *RedisStore) Get(ctx context.Context, email string) (string, error) {
	code, err := s.rdb.Get(ctx, otpdomain.Key(email)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", otpdomain.ErrOTPExpired
		}
		return "", err
	}
	return code, nil
}

func (s *RedisStore) Del(ctx context.Context, email string) error {
	return s.rdb.Del(ctx, otpdomain.Key(email)).Err()
}

func (s *RedisStore) Consume(ctx context.Context, email, code string) (bool, error) {
	result, err := consumeScript.Run(ctx, s.rdb, []string{otpdomain.Key(email)}, code).Int64()
	if err != nil {
		return false, err
	}
	switch result {
	case 2:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, otpdomain.ErrOTPExpired
	}
}

func (s *RedisStore) DelIfMatch(ctx context.Context, email, code string) error {
	return delIfMatchScript.Run(ctx, s.rdb, []string{otpdomain.Key(email)}, code).Err()
}

func (s *RedisStore) SetCooldown(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.rdb.SetNX(ctx, key, "1", ttl).Result()
}

func (s *RedisStore) IncrAttempts(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := incrAttemptsScript.Run(ctx, s.rdb, []string{key}, ttl.Milliseconds()).Int64()
	if err != nil {
		return 0, err
	}
	return n, nil
}
