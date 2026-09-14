package otp

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

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
