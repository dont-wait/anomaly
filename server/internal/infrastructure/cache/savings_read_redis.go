package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type SavingsReadRedis struct{ client *redis.Client }

func NewSavingsReadRedis(client *redis.Client) *SavingsReadRedis {
	return &SavingsReadRedis{client: client}
}

func (r *SavingsReadRedis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if r == nil || r.client == nil {
		return nil, false, errors.New("redis unavailable")
	}
	b, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	return b, err == nil, err
}

func (r *SavingsReadRedis) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	if r == nil || r.client == nil {
		return errors.New("redis unavailable")
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}
