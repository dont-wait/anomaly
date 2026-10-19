package queries

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// MongoDB Reader gia lap.
type repaymentReaderStub struct {
	calls    int
	balance  int64
	schedule Schedule
	payments []Payment
}

func (s *repaymentReaderStub) ReadSchedule(
	_ context.Context,
	_, _ string,
) (Schedule, error) {
	s.calls++
	return s.schedule, nil
}

func (s *repaymentReaderStub) ReadPayments(
	_ context.Context,
	_, _ string,
) ([]Payment, error) {
	s.calls++
	return s.payments, nil
}

func (s *repaymentReaderStub) ReadPayment(
	_ context.Context,
	_, _, _ string,
) (Payment, error) {
	s.calls++

	if len(s.payments) == 0 {
		return Payment{}, ErrPaymentNotFound
	}

	return s.payments[0], nil
}

func (s *repaymentReaderStub) ReadAvailableBalance(
	_ context.Context,
	_, _, _ string,
) (int64, error) {
	return s.balance, nil
}

// Redis Cache gia lap.
type repaymentCacheStub struct {
	items   map[string][]byte
	failSet bool
}

func (s *repaymentCacheStub) Get(
	_ context.Context,
	key string,
) ([]byte, bool, error) {
	data, exists := s.items[key]
	return data, exists, nil
}

func (s *repaymentCacheStub) Set(
	_ context.Context,
	key string,
	data []byte,
	_ time.Duration,
) error {
	if s.failSet {
		return errors.New("redis unavailable")
	}

	s.items[key] = append([]byte(nil), data...)
	return nil
}

func (s *repaymentCacheStub) Delete(
	_ context.Context,
	keys ...string,
) error {
	for _, key := range keys {
		delete(s.items, key)
	}
	return nil
}

// Chuan bi du lieu test.
func repaymentFixture() (
	*RepaymentQueryService,
	*repaymentReaderStub,
	*repaymentCacheStub,
	time.Time,
) {
	vn := time.FixedZone("ICT", 7*3600)

	now := time.Date(
		2026, 11, 14,
		9, 0, 0, 0, vn,
	)

	reader := &repaymentReaderStub{
		balance: 6_000_000,
		schedule: Schedule{
			LoanID:               "loan-001",
			LoanNo:               "loan-001",
			Principal:            50_000_000,
			OutstandingPrincipal: 50_000_000,
			AnnualInterestRate:   "16.30",
			TermMonths:           12,
			RepaymentMethod:      "fixed_total_reducing_balance",
			Status:               "active",
			NextInstallmentNo:    1,
			Items: []Installment{
				{
					InstallmentNo: 1,
					DueDate:       "2026-11-14",
					PrincipalDue:  3_800_000,
					InterestDue:   600_000,
					TotalDue:      4_400_000,
					Status:        "upcoming",
				},
			},
		},
		payments: []Payment{
			{
				PaymentID:     "abcdefabcdefabcdefabcdef",
				LoanID:        "loan-001",
				InstallmentNo: 1,
				Amount:        4_400_000,
			},
		},
	}

	cache := &repaymentCacheStub{
		items: map[string][]byte{},
	}

	service := NewRepaymentQueryService(
		reader, cache,
	)

	service.Clock = func() time.Time {
		return now
	}

	return service, reader, cache, now
}

// Test Cache MISS, HIT va Invalidation.
func TestRepaymentReadCacheHitMiss(t *testing.T) {
	ctx := context.Background()

	service, reader, cache, _ :=
		repaymentFixture()

	first, err := service.Schedule(
		ctx, "loan-001", "owner-001",
	)
	if err != nil ||
		first.Principal != 50_000_000 ||
		reader.calls != 1 {
		t.Fatalf(
			"first read: %+v, %v, calls=%d",
			first, err, reader.calls,
		)
	}

	// Thay du lieu MongoDB gia lap.
	reader.schedule.Principal = 1

	second, err := service.Schedule(
		ctx, "loan-001", "owner-001",
	)
	if err != nil ||
		second.Principal != 50_000_000 ||
		reader.calls != 1 {
		t.Fatal("expected Redis Cache HIT")
	}

	// Xoa cache sau khi Projection.
	if err := service.Invalidate(
		ctx, "owner-001", "loan-001",
	); err != nil {
		t.Fatal(err)
	}

	third, err := service.Schedule(
		ctx, "loan-001", "owner-001",
	)
	if err != nil ||
		third.Principal != 1 ||
		reader.calls != 2 {
		t.Fatal("expected fresh MongoDB read")
	}

	key := RepaymentCacheKey(
		"owner-001", "loan-001", "schedule",
	)

	if _, exists := cache.items[key]; !exists {
		t.Fatal("cache not refreshed")
	}
}

// Cache cua hai khach hang phai doc lap.
func TestRepaymentCacheIsScopedToCustomer(
	t *testing.T,
) {
	service, reader, _, _ := repaymentFixture()

	_, _ = service.Schedule(
		context.Background(),
		"loan-001",
		"owner-001",
	)

	_, _ = service.Schedule(
		context.Background(),
		"loan-001",
		"owner-002",
	)

	if reader.calls != 2 {
		t.Fatal("customers shared cached data")
	}
}

// Quote thanh toan dung so tien va duoc luu.
func TestRepaymentQuoteCorrectInstallmentAndTTL(
	t *testing.T,
) {
	service, _, cache, _ := repaymentFixture()

	ctx := context.Background()

	q, err := service.Quote(
		ctx,
		"loan-001",
		"owner-001",
		"owner-001",
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	if q.TotalAmount != 4_400_000 ||
		q.PrincipalAmount != 3_800_000 ||
		q.InterestAmount != 600_000 ||
		q.PenaltyAmount != 0 ||
		q.QuoteID == "" {
		t.Fatalf("unexpected quote: %+v", q)
	}

	stored, err := service.GetQuote(
		ctx,
		q.QuoteID,
		"owner-001",
	)
	if err != nil ||
		stored.Quote.QuoteID != q.QuoteID {
		t.Fatalf("quote not saved: %v", err)
	}

	// Khach hang khac khong duoc lay Quote.
	_, err = service.GetQuote(
		ctx,
		q.QuoteID,
		"owner-002",
	)
	if !errors.Is(err, ErrQuoteNotFound) {
		t.Fatal("quote ownership leak")
	}

	var decoded StoredQuote

	err = json.Unmarshal(
		cache.items["loan-repayment:quote:"+q.QuoteID],
		&decoded,
	)
	if err != nil ||
		decoded.Quote.TotalAmount != 4_400_000 {
		t.Fatal("invalid Redis quote")
	}
}

// Khong tao Quote sai ky hoac sai ngay.
func TestRepaymentQuoteRejectsWrongDayAndInstallment(
	t *testing.T,
) {
	service, _, _, now := repaymentFixture()
	ctx := context.Background()

	_, err := service.Quote(
		ctx,
		"loan-001",
		"owner-001",
		"owner-001",
		2,
	)

	if !errors.Is(
		err,
		ErrInvalidInstallmentOrder,
	) {
		t.Fatalf("wrong installment: %v", err)
	}

	service.Clock = func() time.Time {
		return now.Add(-24 * time.Hour)
	}

	_, err = service.Quote(
		ctx,
		"loan-001",
		"owner-001",
		"owner-001",
		1,
	)

	if !errors.Is(err, ErrInstallmentNotDue) {
		t.Fatalf("early quote: %v", err)
	}

	service.Clock = func() time.Time {
		return now
	}

	_, err = service.Quote(
		ctx,
		"loan-001",
		"owner-001",
		"other-account",
		1,
	)

	if !errors.Is(
		err,
		ErrInvalidRepaymentQuery,
	) {
		t.Fatalf("wrong account: %v", err)
	}
}

// Kiem tra so du va loi Redis.
func TestRepaymentQuoteInsufficientBalanceAndRedisFailure(
	t *testing.T,
) {
	service, reader, cache, _ :=
		repaymentFixture()

	ctx := context.Background()

	reader.balance = 1

	_, err := service.Quote(
		ctx,
		"loan-001",
		"owner-001",
		"owner-001",
		1,
	)

	if !errors.Is(
		err,
		ErrInsufficientBalance,
	) {
		t.Fatalf("expected low balance: %v", err)
	}

	reader.balance = 6_000_000
	cache.failSet = true

	_, err = service.Quote(
		ctx,
		"loan-001",
		"owner-001",
		"owner-001",
		1,
	)

	if err == nil {
		t.Fatal("unpersisted quote accepted")
	}
}

// Quote het han khong con hop le.
func TestRepaymentQuoteExpires(t *testing.T) {
	service, _, _, now := repaymentFixture()

	ctx := context.Background()

	quote, err := service.Quote(
		ctx,
		"loan-001",
		"owner-001",
		"owner-001",
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	service.Clock = func() time.Time {
		return now.Add(6 * time.Minute)
	}

	_, err = service.GetQuote(
		ctx, quote.QuoteID, "owner-001",
	)

	if !errors.Is(err, ErrQuoteNotFound) {
		t.Fatalf("expired quote accepted: %v", err)
	}
}
