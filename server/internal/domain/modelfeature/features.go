package modelfeature

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	FeatureWindow7Days  = 7 * 24 * time.Hour
	FeatureWindow1Hour  = time.Hour
	FeatureWindow24Hour = 24 * time.Hour
	FeatureBucketSize   = time.Minute
)

// Features are the model features attached to a completed transfer event.
// LastTxAt is the timestamp before the current transaction is applied.
type Features struct {
	AvgAmount7d int64      `json:"avg_amount_7d" bson:"avg_amount_7d"`
	TxCount1h   int64      `json:"tx_count_1h" bson:"tx_count_1h"`
	TxCount24h  int64      `json:"tx_count_24h" bson:"tx_count_24h"`
	LastTxAt    *time.Time `json:"last_tx_at" bson:"last_tx_at,omitempty"`
	NewReceiver bool       `json:"new_receiver" bson:"new_receiver"`
	NewDevice   bool       `json:"new_device" bson:"new_device"`
}

type Transaction struct {
	EventID         string
	SourceAccountID string
	DestinationID   string
	DeviceID        string
	Amount          int64
	OccurredAt      time.Time
}

type Bucket struct {
	StartAt   time.Time `json:"start_at" bson:"start_at"`
	AmountSum int64     `json:"amount_sum" bson:"amount_sum"`
	TxCount   int64     `json:"tx_count" bson:"tx_count"`
}

type State struct {
	AccountID    string     `json:"account_id" bson:"_id"`
	LastTxAt     *time.Time `json:"last_tx_at,omitempty" bson:"last_tx_at,omitempty"`
	Buckets      []Bucket   `json:"rolling_buckets" bson:"rolling_buckets"`
	StateVersion int64      `json:"state_version" bson:"state_version"`
	LastEventID  string     `json:"last_event_id" bson:"last_event_id"`
	UpdatedAt    time.Time  `json:"updated_at" bson:"updated_at"`
}

type Decision struct {
	EventID      string   `json:"event_id" bson:"_id"`
	Features     Features `json:"features" bson:"features"`
	StateApplied bool     `json:"state_applied" bson:"state_applied"`
}

// Calculate derives features from the state before tx and returns the next
// state that should be committed after Kafka accepts the enriched event.
func Calculate(state State, tx Transaction, receiverKnown, deviceKnown bool) (Features, State, error) {
	if strings.TrimSpace(tx.EventID) == "" {
		return Features{}, State{}, fmt.Errorf("event id is required")
	}
	if strings.TrimSpace(tx.SourceAccountID) == "" {
		return Features{}, State{}, fmt.Errorf("source account id is required")
	}
	if strings.TrimSpace(tx.DestinationID) == "" {
		return Features{}, State{}, fmt.Errorf("destination id is required")
	}
	if tx.Amount <= 0 {
		return Features{}, State{}, fmt.Errorf("transaction amount must be positive")
	}
	if tx.OccurredAt.IsZero() {
		return Features{}, State{}, fmt.Errorf("transaction occurred at is required")
	}

	state.AccountID = tx.SourceAccountID
	state.Buckets = pruneBuckets(state.Buckets, tx.OccurredAt)

	features := Features{
		AvgAmount7d: averageAmount(state.Buckets, tx.OccurredAt, FeatureWindow7Days),
		TxCount1h:   countTransactions(state.Buckets, tx.OccurredAt, FeatureWindow1Hour),
		TxCount24h:  countTransactions(state.Buckets, tx.OccurredAt, FeatureWindow24Hour),
		LastTxAt:    cloneTime(state.LastTxAt),
		NewReceiver: !receiverKnown,
		NewDevice:   strings.TrimSpace(tx.DeviceID) != "" && !strings.EqualFold(strings.TrimSpace(tx.DeviceID), "unknown") && !deviceKnown,
	}

	bucketStart := tx.OccurredAt.UTC().Truncate(FeatureBucketSize)
	updatedBuckets := append([]Bucket(nil), state.Buckets...)
	found := false
	for i := range updatedBuckets {
		if updatedBuckets[i].StartAt.Equal(bucketStart) {
			if updatedBuckets[i].AmountSum > math.MaxInt64-tx.Amount {
				return Features{}, State{}, fmt.Errorf("feature amount sum overflow")
			}
			updatedBuckets[i].AmountSum += tx.Amount
			updatedBuckets[i].TxCount++
			found = true
			break
		}
	}
	if !found {
		updatedBuckets = append(updatedBuckets, Bucket{StartAt: bucketStart, AmountSum: tx.Amount, TxCount: 1})
	}
	sort.Slice(updatedBuckets, func(i, j int) bool { return updatedBuckets[i].StartAt.Before(updatedBuckets[j].StartAt) })

	next := State{
		AccountID:    tx.SourceAccountID,
		LastTxAt:     cloneTime(state.LastTxAt),
		Buckets:      updatedBuckets,
		StateVersion: state.StateVersion + 1,
		LastEventID:  tx.EventID,
		UpdatedAt:    time.Now().UTC(),
	}
	if next.LastTxAt == nil || tx.OccurredAt.After(*next.LastTxAt) {
		next.LastTxAt = cloneTime(&tx.OccurredAt)
	}
	return features, next, nil
}

func pruneBuckets(buckets []Bucket, at time.Time) []Bucket {
	cutoff := at.UTC().Add(-FeatureWindow7Days).Truncate(FeatureBucketSize)
	result := make([]Bucket, 0, len(buckets))
	for _, bucket := range buckets {
		if !bucket.StartAt.Before(cutoff) {
			result = append(result, bucket)
		}
	}
	return result
}

func countTransactions(buckets []Bucket, at time.Time, window time.Duration) int64 {
	cutoff := at.UTC().Add(-window)
	var count int64
	for _, bucket := range buckets {
		if !bucket.StartAt.Before(cutoff) {
			count += bucket.TxCount
		}
	}
	return count
}

func averageAmount(buckets []Bucket, at time.Time, window time.Duration) int64 {
	cutoff := at.UTC().Add(-window)
	var sum, count int64
	for _, bucket := range buckets {
		if bucket.StartAt.Before(cutoff) {
			continue
		}
		if sum > math.MaxInt64-bucket.AmountSum || count > math.MaxInt64-bucket.TxCount {
			return 0
		}
		sum += bucket.AmountSum
		count += bucket.TxCount
	}
	if count == 0 {
		return 0
	}
	return sum / count
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
