package transaction

import "time"

type (
	TransactionType   string
	TransactionStatus string
	FeedDirection     string
)

const (
	TransactionTypeTransfer TransactionType = "transfer"

	TransactionStatusPosted TransactionStatus = "posted"

	FeedDirectionIn  FeedDirection = "in"
	FeedDirectionOut FeedDirection = "out"
)

type Transaction struct {
	Id              string
	TransactionNo   string
	Type            TransactionType
	SourceAccountId string
	SourceAccountNo string
	DestAccountId   string
	DestAccountNo   string
	Amount          int64
	Currency        string
	Status          TransactionStatus
	IdempotencyKey  string
	CreatedAt       time.Time
	PostedAt        time.Time
}

// FeedEntry là 1 dòng trong danh sách "Giao dịch gần đây" của 1 account cụ
// thể — mỗi Transaction sinh ra 2 FeedEntry (1 cho bên gửi, 1 cho bên nhận).
type FeedEntry struct {
	Id               string
	AccountId        string
	TransactionId    string
	Direction        FeedDirection
	Type             TransactionType
	CounterpartyName string
	CounterpartyNo   string
	Amount           int64
	BalanceAfter     int64
	OccurredAt       time.Time
}
