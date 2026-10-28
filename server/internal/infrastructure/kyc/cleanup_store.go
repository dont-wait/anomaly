package kyc

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type MediaCleanup struct {
	ID        string   `json:"id"`
	AccountID string   `json:"accountId"`
	Keys      []string `json:"keys"`
}

func (s *RealtimeStore) QueueCleanup(ctx context.Context, accountID string, keys []string) error {
	record := MediaCleanup{ID: uuid.NewString(), AccountID: accountID, Keys: keys}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.DB.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, "kyc:cleanup:records", record.ID, data)
		pipe.ZAdd(ctx, "kyc:cleanup:due", redis.Z{Score: float64(time.Now().Add(5 * time.Minute).Unix()), Member: record.ID})
		return nil
	})
	return err
}

func (s *RealtimeStore) PendingCleanup(ctx context.Context) ([]MediaCleanup, error) {
	ids, err := s.DB.ZRangeByScore(ctx, "kyc:cleanup:due", &redis.ZRangeBy{Min: "-inf", Max: strconv.FormatInt(time.Now().Unix(), 10), Offset: 0, Count: 25}).Result()
	if err != nil {
		return nil, err
	}
	var records []MediaCleanup
	for _, id := range ids {
		data, err := s.DB.HGet(ctx, "kyc:cleanup:records", id).Bytes()
		if err != nil {
			return nil, err
		}
		var record MediaCleanup
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *RealtimeStore) FinishCleanup(ctx context.Context, id string) error {
	_, err := s.DB.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HDel(ctx, "kyc:cleanup:records", id)
		pipe.ZRem(ctx, "kyc:cleanup:due", id)
		return nil
	})
	return err
}
