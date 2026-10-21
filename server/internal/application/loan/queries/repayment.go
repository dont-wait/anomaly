package queries

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
	"github.com/google/uuid"
)

var (
	ErrLoanNotFound = errors.New(
		"loan not found",
	)
	ErrPaymentNotFound = errors.New(
		"loan payment not found",
	)
	ErrInvalidRepaymentQuery = errors.New(
		"invalid loan repayment query",
	)
	ErrInstallmentNotDue = errors.New(
		"installment is not due today",
	)
	ErrInstallmentAlreadyPaid = errors.New(
		"installment already paid",
	)
	ErrInvalidInstallmentOrder = errors.New(
		"installment must be paid in order",
	)
	ErrInsufficientBalance = errors.New(
		"insufficient balance",
	)
	ErrQuoteNotFound = errors.New(
		"repayment quote expired or not found",
	)
)

const (
	readTTL  = 15 * time.Second
	quoteTTL = 5 * time.Minute
)

// Thong tin mot ky tra no trong Read Model.
type Installment struct {
	InstallmentNo int    `json:"installmentNo"`
	DueDate       string `json:"dueDate"`
	PrincipalDue  int64  `json:"principalDue"`
	InterestDue   int64  `json:"interestDue"`
	TotalDue      int64  `json:"totalDue"`
	Status        string `json:"status"`
}

// Lich tra no cua khoan vay.
type Schedule struct {
	LoanID               string        `json:"loanId"`
	LoanNo               string        `json:"loanNo"`
	Principal            int64         `json:"principal"`
	OutstandingPrincipal int64         `json:"outstandingPrincipal"`
	AnnualInterestRate   string        `json:"annualInterestRate"`
	TermMonths           int           `json:"termMonths"`
	RepaymentMethod      string        `json:"repaymentMethod"`
	Status               string        `json:"status"`
	Items                []Installment `json:"items"`
	NextInstallmentNo    int           `json:"nextInstallmentNo"`
}

// Lich su mot lan thanh toan.
type Payment struct {
	PaymentID       string    `json:"paymentId"`
	LoanID          string    `json:"loanId"`
	TransactionID   string    `json:"transactionId"`
	InstallmentNo   int       `json:"installmentNo"`
	Amount          int64     `json:"amount"`
	PrincipalAmount int64     `json:"principalAmount"`
	InterestAmount  int64     `json:"interestAmount"`
	PenaltyAmount   int64     `json:"penaltyAmount"`
	Channel         string    `json:"channel"`
	Status          string    `json:"status"`
	PaidAt          time.Time `json:"paidAt"`
}

// Bao gia thanh toan ky vay.
type Quote struct {
	QuoteID         string `json:"quoteId"`
	LoanID          string `json:"loanId"`
	InstallmentNo   int    `json:"installmentNo"`
	DueDate         string `json:"dueDate"`
	PrincipalAmount int64  `json:"principalAmount"`
	InterestAmount  int64  `json:"interestAmount"`
	PenaltyAmount   int64  `json:"penaltyAmount"`
	TotalAmount     int64  `json:"totalAmount"`
	Currency        string `json:"currency"`

	SourceAccount struct {
		AccountID        string `json:"accountId"`
		AvailableBalance int64  `json:"availableBalance"`
	} `json:"sourceAccount"`

	QuoteExpiresAt time.Time `json:"quoteExpiresAt"`
}

// Quote tam thoi luu trong Redis.
// Khong phai bang chung da thanh toan.
type StoredQuote struct {
	CustomerID      string `json:"customerId"`
	SourceAccountID string `json:"sourceAccountId"`
	Quote           Quote  `json:"quote"`
}

// Port doc du lieu tu MongoDB.
type RepaymentReader interface {
	ReadSchedule(
		context.Context, string, string,
	) (Schedule, error)

	ReadPayments(
		context.Context, string, string,
	) ([]Payment, error)

	ReadPayment(
		context.Context, string, string, string,
	) (Payment, error)

	ReadAvailableBalance(
		context.Context, string, string, string,
	) (int64, error)
}

// Port Redis Cache.
type RepaymentCache interface {
	Get(
		context.Context, string,
	) ([]byte, bool, error)

	Set(
		context.Context, string, []byte, time.Duration,
	) error

	Delete(context.Context, ...string) error
	// Generation and conditional SET prevent stale reads from racing DEL.
	Version(context.Context, string) (int64, error)
	SetIfVersion(context.Context, string, []byte, time.Duration, int64) (bool, error)
	InvalidateReadKeys(context.Context, ...string) error
}

type RepaymentQueryService struct {
	Reader RepaymentReader
	Cache  RepaymentCache
	Clock  func() time.Time
}

func NewRepaymentQueryService(
	reader RepaymentReader,
	cache RepaymentCache,
) *RepaymentQueryService {
	return &RepaymentQueryService{
		Reader: reader,
		Cache:  cache,
	}
}

func (s *RepaymentQueryService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}

	return time.Now().UTC()
}

func validIDs(loanID, customerID string) bool {
	return loanID != "" && customerID != ""
}

// Cache key co CustomerID de tranh dung chung
// du lieu giua cac khach hang.
func RepaymentCacheKey(
	customerID, loanID, kind string,
) string {
	hash := sha256.Sum256(
		[]byte(customerID + ":" + loanID),
	)

	return "loan-repayment:v1:" +
		hex.EncodeToString(hash[:]) +
		":" + kind
}

// Xu ly Redis HIT/MISS.
//
// Neu Redis doc bi loi, fallback sang MongoDB.
// Khong cache du lieu khi MongoDB doc that bai.
func (s *RepaymentQueryService) cached(
	ctx context.Context,
	key string,
	dest any,
	load func() error,
) error {
	if s.Cache != nil {
		data, found, err := s.Cache.Get(ctx, key)

		if err == nil && found {
			if json.Unmarshal(data, dest) == nil {
				return nil
			}
		}
	}

	// Capture generation BEFORE reading MongoDB. A projector can invalidate
	// during load; its bump and our conditional SET are atomic in Redis.
	var generation int64
	var canCache bool
	if s.Cache != nil {
		var err error
		generation, err = s.Cache.Version(ctx, key)
		canCache = err == nil
	}

	if err := load(); err != nil {
		return err
	}

	if canCache {
		data, err := json.Marshal(dest)
		if err == nil {
			_, _ = s.Cache.SetIfVersion(ctx, key, data, readTTL, generation)
		}
	}

	return nil
}

// GET lich tra no.
func (s *RepaymentQueryService) Schedule(
	ctx context.Context,
	loanID, customerID string,
) (Schedule, error) {
	if s == nil ||
		s.Reader == nil ||
		!validIDs(loanID, customerID) {
		return Schedule{},
			ErrInvalidRepaymentQuery
	}

	var result Schedule

	err := s.cached(
		ctx,
		RepaymentCacheKey(
			customerID, loanID, "schedule",
		),
		&result,
		func() error {
			var err error
			result, err = s.Reader.ReadSchedule(
				ctx, loanID, customerID,
			)
			return err
		},
	)

	return result, err
}

// GET danh sach cac lan tra no.
func (s *RepaymentQueryService) Payments(
	ctx context.Context,
	loanID, customerID string,
) ([]Payment, error) {
	if s == nil ||
		s.Reader == nil ||
		!validIDs(loanID, customerID) {
		return nil, ErrInvalidRepaymentQuery
	}

	var result []Payment

	err := s.cached(
		ctx,
		RepaymentCacheKey(
			customerID, loanID, "payments",
		),
		&result,
		func() error {
			var err error
			result, err = s.Reader.ReadPayments(
				ctx, loanID, customerID,
			)
			return err
		},
	)

	return result, err
}

// GET chi tiet mot lan thanh toan.
func (s *RepaymentQueryService) Payment(
	ctx context.Context,
	loanID, customerID, paymentID string,
) (Payment, error) {
	if s == nil ||
		s.Reader == nil ||
		!validIDs(loanID, customerID) ||
		paymentID == "" {
		return Payment{},
			ErrInvalidRepaymentQuery
	}

	var result Payment

	err := s.cached(
		ctx,
		RepaymentCacheKey(
			customerID,
			loanID,
			"payment:"+paymentID,
		),
		&result,
		func() error {
			var err error
			result, err = s.Reader.ReadPayment(
				ctx,
				loanID,
				customerID,
				paymentID,
			)
			return err
		},
	)

	return result, err
}

// So sanh dung ngay den han theo gio Viet Nam.
func dueOnDate(
	dueDate string,
	at time.Time,
) bool {
	vn := timeutil.VietnamLocation

	return at.In(vn).Format("2006-01-02") ==
		dueDate
}

// Tao bao gia cho dung ky thanh toan.
//
// Quote lay du lieu moi tu MongoDB.
// Khi thanh toan that su, Banking phai
// kiem tra lai so du va tinh hop le.
func (s *RepaymentQueryService) Quote(
	ctx context.Context,
	loanID, customerID string,
	sourceAccountID string,
	installmentNo int,
) (Quote, error) {
	if s == nil ||
		s.Reader == nil ||
		s.Cache == nil ||
		!validIDs(loanID, customerID) ||
		sourceAccountID == "" ||
		installmentNo <= 0 {
		return Quote{},
			ErrInvalidRepaymentQuery
	}

	schedule, err := s.Reader.ReadSchedule(
		ctx, loanID, customerID,
	)
	if err != nil {
		return Quote{}, err
	}

	if schedule.Status != "active" ||
		installmentNo < schedule.NextInstallmentNo {
		return Quote{},
			ErrInstallmentAlreadyPaid
	}

	if installmentNo != schedule.NextInstallmentNo ||
		installmentNo > len(schedule.Items) {
		return Quote{},
			ErrInvalidInstallmentOrder
	}

	item := schedule.Items[installmentNo-1]

	if item.InstallmentNo != installmentNo ||
		item.Status == "paid" {
		return Quote{},
			ErrInstallmentAlreadyPaid
	}

	if !dueOnDate(item.DueDate, s.now()) {
		return Quote{},
			ErrInstallmentNotDue
	}

	balance, err := s.Reader.ReadAvailableBalance(
		ctx,
		loanID,
		customerID,
		sourceAccountID,
	)
	if err != nil {
		return Quote{}, err
	}

	if balance < item.TotalDue {
		return Quote{},
			ErrInsufficientBalance
	}

	var result Quote

	result.QuoteID = uuid.NewString()
	result.LoanID = loanID
	result.InstallmentNo = installmentNo
	result.DueDate = item.DueDate
	result.PrincipalAmount = item.PrincipalDue
	result.InterestAmount = item.InterestDue
	result.PenaltyAmount = 0
	result.TotalAmount = item.TotalDue
	result.Currency = "VND"
	result.SourceAccount.AccountID = sourceAccountID
	result.SourceAccount.AvailableBalance = balance
	result.QuoteExpiresAt = s.now().Add(quoteTTL)

	stored := StoredQuote{
		CustomerID:      customerID,
		SourceAccountID: sourceAccountID,
		Quote:           result,
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return Quote{}, err
	}

	// Quote can luu thanh cong vao Redis.
	if err := s.Cache.Set(
		ctx,
		"loan-repayment:quote:"+result.QuoteID,
		data,
		quoteTTL,
	); err != nil {
		return Quote{}, fmt.Errorf(
			"save repayment quote: %w", err,
		)
	}

	return result, nil
}

// Doc lai Quote khi khach hang xac nhan.
// Giai doan 5 se dung ham nay.
func (s *RepaymentQueryService) GetQuote(
	ctx context.Context,
	quoteID, customerID string,
) (StoredQuote, error) {
	if s == nil ||
		s.Cache == nil ||
		quoteID == "" ||
		customerID == "" {
		return StoredQuote{},
			ErrInvalidRepaymentQuery
	}

	data, found, err := s.Cache.Get(
		ctx,
		"loan-repayment:quote:"+quoteID,
	)
	if err != nil {
		return StoredQuote{}, err
	}

	if !found {
		return StoredQuote{}, ErrQuoteNotFound
	}

	var stored StoredQuote

	if json.Unmarshal(data, &stored) != nil ||
		stored.CustomerID != customerID ||
		stored.Quote.QuoteID != quoteID ||
		!s.now().Before(
			stored.Quote.QuoteExpiresAt,
		) {
		return StoredQuote{}, ErrQuoteNotFound
	}

	return stored, nil
}

// Xoa cache cua khoan vay sau khi
// MongoDB Projection cap nhat thanh cong.
func (s *RepaymentQueryService) Invalidate(
	ctx context.Context,
	customerID, loanID string,
) error {
	if s == nil || s.Cache == nil {
		return nil
	}

	return s.Cache.InvalidateReadKeys(
		ctx,
		RepaymentCacheKey(
			customerID, loanID, "schedule",
		),
		RepaymentCacheKey(
			customerID, loanID, "payments",
		),
	)
}
