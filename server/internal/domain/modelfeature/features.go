package modelfeature

import (
	"fmt"
	"math"
	"math/big"
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
	StreamRevision  uint64
	SourceAccountID string
	DestinationID   string
	DeviceID        string
	Amount          int64
	OccurredAt      time.Time
}

type Bucket struct {
	StartAt        time.Time           `json:"start_at" bson:"start_at"`
	AmountSum      int64               `json:"amount_sum" bson:"amount_sum"`
	AmountSumExact string              `json:"amount_sum_exact,omitempty" bson:"amount_sum_exact,omitempty"`
	TxCount        int64               `json:"tx_count" bson:"tx_count"`
	Samples        []TransactionSample `json:"samples,omitempty" bson:"samples,omitempty"`
}

// TransactionSample keeps enough detail inside the bounded rolling state to
// calculate windows exactly at minute boundaries and to ignore future events
// when historical events arrive out of order.
type TransactionSample struct {
	OccurredAt time.Time `json:"occurred_at" bson:"occurred_at"`
	Amount     int64     `json:"amount" bson:"amount"`
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
		LastTxAt:    lastTransactionAt(state.Buckets, state.LastTxAt, tx.OccurredAt),
		NewReceiver: !receiverKnown,
		NewDevice:   strings.TrimSpace(tx.DeviceID) != "" && !strings.EqualFold(strings.TrimSpace(tx.DeviceID), "unknown") && !deviceKnown,
	}

	bucketStart := tx.OccurredAt.UTC().Truncate(FeatureBucketSize)
	updatedBuckets := append([]Bucket(nil), state.Buckets...)
	found := false
	for i := range updatedBuckets {
		if updatedBuckets[i].StartAt.Equal(bucketStart) {
			amount := bucketAmount(updatedBuckets[i])
			amount.Add(amount, big.NewInt(tx.Amount))
			setBucketAmount(&updatedBuckets[i], amount)
			updatedBuckets[i].TxCount = saturatingAdd(updatedBuckets[i].TxCount, 1)
			updatedBuckets[i].Samples = append(updatedBuckets[i].Samples, TransactionSample{
				OccurredAt: tx.OccurredAt.UTC(),
				Amount:     tx.Amount,
			})
			found = true
			break
		}
	}
	if !found {
		updatedBuckets = append(updatedBuckets, Bucket{
			StartAt:   bucketStart,
			AmountSum: tx.Amount,
			TxCount:   1,
			Samples: []TransactionSample{{
				OccurredAt: tx.OccurredAt.UTC(),
				Amount:     tx.Amount,
			}},
		})
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
	cutoff := at.UTC().Add(-FeatureWindow7Days)
	result := make([]Bucket, 0, len(buckets))
	for _, bucket := range buckets {
		if int64(len(bucket.Samples)) == bucket.TxCount && len(bucket.Samples) > 0 {
			filtered := bucket
			filtered.Samples = filtered.Samples[:0]
			filtered.AmountSum = 0
			filtered.AmountSumExact = ""
			filtered.TxCount = 0
			for _, sample := range bucket.Samples {
				if sample.OccurredAt.Before(cutoff) {
					continue
				}
				amount := bucketAmount(filtered)
				amount.Add(amount, big.NewInt(sample.Amount))
				setBucketAmount(&filtered, amount)
				filtered.TxCount = saturatingAdd(filtered.TxCount, 1)
				filtered.Samples = append(filtered.Samples, sample)
			}
			if filtered.TxCount > 0 {
				result = append(result, filtered)
			}
			continue
		}
		if !bucket.StartAt.Before(cutoff.Truncate(FeatureBucketSize)) {
			result = append(result, bucket)
		}
	}
	return result
}

func countTransactions(buckets []Bucket, at time.Time, window time.Duration) int64 {
	cutoff := at.UTC().Add(-window)
	var count int64
	for _, bucket := range buckets {
		if int64(len(bucket.Samples)) == bucket.TxCount && len(bucket.Samples) > 0 {
			for _, sample := range bucket.Samples {
				if !sample.OccurredAt.Before(cutoff) && !sample.OccurredAt.After(at) {
					count = saturatingAdd(count, 1)
				}
			}
			continue
		}
		if !bucket.StartAt.Before(cutoff) && !bucket.StartAt.After(at) {
			count = saturatingAdd(count, bucket.TxCount)
		}
	}
	return count
}

func averageAmount(buckets []Bucket, at time.Time, window time.Duration) int64 {
	cutoff := at.UTC().Add(-window)
	sum := new(big.Int)
	var count int64
	for _, bucket := range buckets {
		if int64(len(bucket.Samples)) == bucket.TxCount && len(bucket.Samples) > 0 {
			for _, sample := range bucket.Samples {
				if !sample.OccurredAt.Before(cutoff) && !sample.OccurredAt.After(at) {
					sum.Add(sum, big.NewInt(sample.Amount))
					count = saturatingAdd(count, 1)
				}
			}
			continue
		}
		if !bucket.StartAt.Before(cutoff) && !bucket.StartAt.After(at) {
			sum.Add(sum, bucketAmount(bucket))
			count = saturatingAdd(count, bucket.TxCount)
		}
	}
	if count == 0 {
		return 0
	}
	return new(big.Int).Div(sum, big.NewInt(count)).Int64()
}

func lastTransactionAt(buckets []Bucket, fallback *time.Time, at time.Time) *time.Time {
	var latest *time.Time
	for _, bucket := range buckets {
		for _, sample := range bucket.Samples {
			if sample.OccurredAt.After(at) {
				continue
			}
			candidate := sample.OccurredAt.UTC()
			if latest == nil || candidate.After(*latest) {
				latest = &candidate
			}
		}
	}
	if latest != nil {
		return latest
	}
	if fallback != nil && !fallback.After(at) {
		return cloneTime(fallback)
	}
	return nil
}

func bucketAmount(bucket Bucket) *big.Int {
	if bucket.AmountSumExact != "" {
		if amount, ok := new(big.Int).SetString(bucket.AmountSumExact, 10); ok {
			return amount
		}
	}
	return big.NewInt(bucket.AmountSum)
}

func setBucketAmount(bucket *Bucket, amount *big.Int) {
	if amount.IsInt64() {
		bucket.AmountSum = amount.Int64()
		bucket.AmountSumExact = ""
		return
	}
	bucket.AmountSum = math.MaxInt64
	bucket.AmountSumExact = amount.String()
}

func saturatingAdd(left, right int64) int64 {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
