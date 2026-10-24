package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// SavingsQuoteRedis stores server-authoritative quotes. 5 minutes is the
// *business* TTL; an extra minute of retention distinguishes an expired quote
// from a never-issued ID for the confirmation endpoint in GĐ3.
type SavingsQuoteRedis struct{ client *redis.Client }

func NewSavingsQuoteRedis(client *redis.Client) *SavingsQuoteRedis {
	return &SavingsQuoteRedis{client: client}
}

func savingsQuoteKey(id string) string { return "savings:quote:" + id }

func (s *SavingsQuoteRedis) Save(ctx context.Context, q savingsqueries.Quote) error {
	if s == nil || s.client == nil || q.ID == "" || q.ExpiresAt.IsZero() {
		return errors.New("invalid savings quote store or value")
	}
	if _, err := uuid.Parse(q.ID); err != nil {
		return fmt.Errorf("invalid savings quote ID: %w", err)
	}
	data, err := json.Marshal(q)
	if err != nil {
		return err
	}
	// Stable retention clock: a quote expires logically at ExpiresAt, and
	// cannot be used after that time even if Redis still holds its payload.
	ok, err := s.client.SetNX(ctx, savingsQuoteKey(q.ID), data, savings.QuoteTTL+time.Minute).Result()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("duplicate savings quote ID")
	}
	return nil
}

func (s *SavingsQuoteRedis) Get(ctx context.Context, id string) (savingsqueries.Quote, bool, error) {
	if s == nil || s.client == nil {
		return savingsqueries.Quote{}, false, errors.New("savings quote store unavailable")
	}
	if _, err := uuid.Parse(id); err != nil {
		return savingsqueries.Quote{}, false, nil
	}
	data, err := s.client.Get(ctx, savingsQuoteKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return savingsqueries.Quote{}, false, nil
	}
	if err != nil {
		return savingsqueries.Quote{}, false, err
	}
	var q savingsqueries.Quote
	if err := json.Unmarshal(data, &q); err != nil {
		return savingsqueries.Quote{}, false, fmt.Errorf("corrupt stored savings quote: %w", err)
	}
	if q.ID != id || q.ExpiresAt.IsZero() || q.ActorID == "" || q.SourceAccountID == "" {
		return savingsqueries.Quote{}, false, errors.New("stored savings quote is invalid")
	}
	return q, true, nil
}
