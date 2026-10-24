package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	savingscommands "github.com/dont-wait/anomaly/internal/application/savings/commands"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type SavingsWithdrawalQuoteRedis struct{ client *redis.Client }

func NewSavingsWithdrawalQuoteRedis(client *redis.Client) *SavingsWithdrawalQuoteRedis {
	return &SavingsWithdrawalQuoteRedis{client: client}
}

func (s *SavingsWithdrawalQuoteRedis) SaveWithdrawal(ctx context.Context, q savingscommands.WithdrawalQuote) error {
	if s == nil || s.client == nil || q.ActorID == "" || q.SavingsID == "" || q.ExpiresAt.IsZero() {
		return errors.New("invalid savings withdrawal quote")
	}
	if _, err := uuid.Parse(q.ID); err != nil {
		return err
	}
	raw, err := json.Marshal(q)
	if err != nil {
		return err
	}
	ok, err := s.client.SetNX(ctx, "savings:withdraw-quote:"+q.ID, raw, savings.QuoteTTL+time.Minute).Result()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("duplicate savings withdrawal quote")
	}
	return nil
}

func (s *SavingsWithdrawalQuoteRedis) GetWithdrawal(ctx context.Context, id string) (savingscommands.WithdrawalQuote, bool, error) {
	if s == nil || s.client == nil {
		return savingscommands.WithdrawalQuote{}, false, errors.New("quote store unavailable")
	}
	if _, err := uuid.Parse(id); err != nil {
		return savingscommands.WithdrawalQuote{}, false, nil
	}
	data, err := s.client.Get(ctx, "savings:withdraw-quote:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return savingscommands.WithdrawalQuote{}, false, nil
	}
	if err != nil {
		return savingscommands.WithdrawalQuote{}, false, err
	}
	var q savingscommands.WithdrawalQuote
	if err := json.Unmarshal(data, &q); err != nil {
		return q, false, err
	}
	if q.ID != id || q.ActorID == "" || q.SavingsID == "" {
		return q, false, errors.New("corrupt withdrawal quote")
	}
	return q, true, nil
}
