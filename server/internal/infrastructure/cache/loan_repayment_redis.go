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
	return r.InvalidateReadKeys(
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

// Version is a monotonic Redis generation for each read-cache key.
func (r *LoanRepaymentRedis) Version(ctx context.Context, key string) (int64, error) {
	v, err := r.client.Get(ctx, key+":generation").Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return v, err
}

// SET and generation comparison must happen in one Redis operation.
var repaymentConditionalSet = redis.NewScript(`
local current = tonumber(redis.call('GET', KEYS[2]) or '0')
if current ~= tonumber(ARGV[3]) then return 0 end
redis.call('PSETEX', KEYS[1], ARGV[2], ARGV[1])
return 1
`)

func (r *LoanRepaymentRedis) SetIfVersion(
	ctx context.Context, key string, data []byte, ttl time.Duration, version int64,
) (bool, error) {
	if ttl <= 0 {
		return false, errors.New("cache TTL must be positive")
	}
	n, err := repaymentConditionalSet.Run(ctx, r.client, []string{key, key + ":generation"},
		data, ttl.Milliseconds(), version).Int64()
	return n == 1, err
}

var repaymentInvalidateReadKeys = redis.NewScript(`
for _, key in ipairs(KEYS) do
  redis.call('INCR', key .. ':generation')
  redis.call('DEL', key)
end
return #KEYS
`)

// InvalidateReadKeys atomically increments generation BEFORE allowing a
// concurrent reader to store its now-obsolete MongoDB snapshot.
func (r *LoanRepaymentRedis) InvalidateReadKeys(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return repaymentInvalidateReadKeys.Run(ctx, r.client, keys).Err()
}
