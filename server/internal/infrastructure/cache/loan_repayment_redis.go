package cache

import (
	"context"
	"errors"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"

	"github.com/redis/go-redis/v9"
)

type LoanRepaymentRedis struct {
	client *redis.Client
}

func NewLoanRepaymentRedis(
	client *redis.Client,
) *LoanRepaymentRedis {
	return &LoanRepaymentRedis{
		client: client,
	}
}

// Redis GET.
func (r *LoanRepaymentRedis) Get(
	ctx context.Context,
	key string,
) ([]byte, bool, error) {
	data, err := r.client.Get(
		ctx, key,
	).Bytes()

	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	return data, true, nil
}

// Redis SET voi TTL.
func (r *LoanRepaymentRedis) Set(
	ctx context.Context,
	key string,
	data []byte,
	ttl time.Duration,
) error {
	return r.client.Set(
		ctx, key, data, ttl,
	).Err()
}

// Redis DEL.
func (r *LoanRepaymentRedis) Delete(
	ctx context.Context,
	keys ...string,
) error {
	if len(keys) == 0 {
		return nil
	}

	return r.client.Del(
		ctx, keys...,
	).Err()
}

// Xoa cache lich tra no va lich su thanh toan.
func (r *LoanRepaymentRedis) Invalidate(
	ctx context.Context,
	customerID, loanID string,
) error {
	return r.Delete(
		ctx,
		loanqueries.RepaymentCacheKey(
			customerID, loanID, "schedule",
		),
		loanqueries.RepaymentCacheKey(
			customerID, loanID, "payments",
		),
	)
}

// Tao Idempotency-Key neu chua ton tai.
// Dung Redis SET NX de tranh hai request
// ghi de cung mot khoa.
func (r *LoanRepaymentRedis) SetNX(
	ctx context.Context,
	key string,
	data []byte,
	ttl time.Duration,
) (bool, error) {
	return r.client.SetNX(
		ctx,
		key,
		data,
		ttl,
	).Result()
}
