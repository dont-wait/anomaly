package modelfeature

import (
	"math"
	"testing"
	"time"
)

func TestCalculateUsesPreviousStateAndUpdatesRollingState(t *testing.T) {
	base := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	previous := base.Add(-30 * time.Minute)
	state := State{
		AccountID: "source-1",
		LastTxAt:  &previous,
		Buckets: []Bucket{
			{StartAt: previous.Truncate(FeatureBucketSize), AmountSum: 100, TxCount: 1},
		},
		StateVersion: 4,
	}

	features, next, err := Calculate(state, Transaction{
		EventID:         "event-2",
		SourceAccountID: "source-1",
		DestinationID:   "receiver-2",
		DeviceID:        "device-2",
		Amount:          300,
		OccurredAt:      base,
	}, false, false)
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if features.AvgAmount7d != 100 || features.TxCount1h != 1 || features.TxCount24h != 1 {
		t.Fatalf("features = %#v, want previous transaction only", features)
	}
	if features.LastTxAt == nil || !features.LastTxAt.Equal(previous) || !features.NewReceiver || !features.NewDevice {
		t.Fatalf("features metadata = %#v", features)
	}
	if next.StateVersion != 5 || next.LastEventID != "event-2" || next.LastTxAt == nil || !next.LastTxAt.Equal(base) {
		t.Fatalf("next state = %#v", next)
	}
	if len(next.Buckets) != 2 {
		t.Fatalf("next buckets = %#v, want two buckets", next.Buckets)
	}
}

func TestCalculatePrunesSevenDayWindow(t *testing.T) {
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	old := now.Add(-FeatureWindow7Days - time.Hour)
	state := State{Buckets: []Bucket{
		{StartAt: old, AmountSum: 1000, TxCount: 1},
	}}

	features, next, err := Calculate(state, Transaction{
		EventID:         "event-1",
		SourceAccountID: "source-1",
		DestinationID:   "receiver-1",
		Amount:          50,
		OccurredAt:      now,
	}, true, true)
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if features.AvgAmount7d != 0 || features.TxCount1h != 0 || features.TxCount24h != 0 {
		t.Fatalf("expired features = %#v", features)
	}
	if len(next.Buckets) != 1 || next.Buckets[0].AmountSum != 50 {
		t.Fatalf("next buckets = %#v, want only current transaction", next.Buckets)
	}
}

func TestCalculateUnknownDeviceDoesNotLookNew(t *testing.T) {
	features, _, err := Calculate(State{}, Transaction{
		EventID:         "event-1",
		SourceAccountID: "source-1",
		DestinationID:   "receiver-1",
		DeviceID:        "UNKNOWN",
		Amount:          50,
		OccurredAt:      time.Now().UTC(),
	}, true, false)
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if features.NewDevice {
		t.Fatal("NewDevice = true for unknown device")
	}
}

func TestCalculateExcludesFutureSamplesFromHistoricalEvent(t *testing.T) {
	base := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	future := base.Add(2 * time.Hour)
	state := State{
		LastTxAt: &future,
		Buckets: []Bucket{{
			StartAt:   future.Truncate(FeatureBucketSize),
			AmountSum: 900,
			TxCount:   1,
			Samples:   []TransactionSample{{OccurredAt: future, Amount: 900}},
		}},
	}

	features, _, err := Calculate(state, Transaction{
		EventID:         "historical-event",
		SourceAccountID: "source-1",
		DestinationID:   "receiver-1",
		Amount:          100,
		OccurredAt:      base,
	}, false, false)
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if features.TxCount1h != 0 || features.AvgAmount7d != 0 || features.LastTxAt != nil {
		t.Fatalf("future activity leaked into historical features: %#v", features)
	}
}

func TestCalculateUsesExactSampleWindowBoundaries(t *testing.T) {
	base := time.Date(2026, 10, 19, 10, 0, 30, 0, time.UTC)
	state := State{Buckets: []Bucket{{
		StartAt:   base.Truncate(FeatureBucketSize),
		AmountSum: 300,
		TxCount:   2,
		Samples: []TransactionSample{
			{OccurredAt: base.Add(-time.Hour - time.Second), Amount: 100},
			{OccurredAt: base.Add(-time.Hour + time.Second), Amount: 200},
		},
	}}}

	features, _, err := Calculate(state, Transaction{
		EventID:         "boundary-event",
		SourceAccountID: "source-1",
		DestinationID:   "receiver-1",
		Amount:          50,
		OccurredAt:      base,
	}, true, true)
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if features.TxCount1h != 1 || features.AvgAmount7d != 150 {
		t.Fatalf("boundary features = %#v, want one exact in-window sample", features)
	}
}

func TestCalculateDoesNotFailWhenBucketAmountExceedsInt64(t *testing.T) {
	base := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	state := State{Buckets: []Bucket{{
		StartAt:   base,
		AmountSum: math.MaxInt64,
		TxCount:   1,
		Samples:   []TransactionSample{{OccurredAt: base, Amount: math.MaxInt64}},
	}}}

	features, next, err := Calculate(state, Transaction{
		EventID:         "large-event",
		SourceAccountID: "source-1",
		DestinationID:   "receiver-1",
		Amount:          math.MaxInt64,
		OccurredAt:      base,
	}, true, true)
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if features.AvgAmount7d != math.MaxInt64 || next.Buckets[0].AmountSumExact == "" {
		t.Fatalf("large amount state = %#v, features = %#v", next.Buckets[0], features)
	}
}

func TestCalculatePreservesNewerStateWhenHistoricalEventArrives(t *testing.T) {
	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	newer := base.Add(10 * 24 * time.Hour)
	state := State{
		AccountID: "source-1",
		LastTxAt:  &newer,
		Buckets: []Bucket{{
			StartAt:   newer.Truncate(FeatureBucketSize),
			AmountSum: 900,
			TxCount:   1,
			Samples:   []TransactionSample{{OccurredAt: newer, Amount: 900}},
		}},
	}

	features, next, err := Calculate(state, Transaction{
		EventID:         "historical-event",
		SourceAccountID: "source-1",
		DestinationID:   "receiver-1",
		Amount:          100,
		OccurredAt:      base,
	}, true, true)
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if features.TxCount24h != 0 || features.AvgAmount7d != 0 || features.LastTxAt != nil {
		t.Fatalf("newer activity leaked into historical features: %#v", features)
	}
	if len(next.Buckets) != 1 || next.Buckets[0].AmountSum != 900 {
		t.Fatalf("historical event erased newer state: %#v", next.Buckets)
	}
	if next.LastTxAt == nil || !next.LastTxAt.Equal(newer) {
		t.Fatalf("LastTxAt = %v, want %v", next.LastTxAt, newer)
	}
}
