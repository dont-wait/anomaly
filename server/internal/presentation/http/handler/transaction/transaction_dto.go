package transaction

import txdomain "github.com/dont-wait/anomaly/internal/domain/transaction"

type FeedEntryResponse struct {
	Id               string `json:"id"`
	TransactionId    string `json:"transactionId"`
	Direction        string `json:"direction"`
	Type             string `json:"type"`
	CounterpartyName string `json:"counterpartyName"`
	CounterpartyNo   string `json:"counterpartyNo"`
	Amount           int64  `json:"amount"`
	BalanceAfter     int64  `json:"balanceAfter"`
	OccurredAt       string `json:"occurredAt"`
}

func toFeedEntryResponse(f *txdomain.FeedEntry) FeedEntryResponse {
	return FeedEntryResponse{
		Id: f.Id, TransactionId: f.TransactionId,
		Direction: string(f.Direction), Type: string(f.Type),
		CounterpartyName: f.CounterpartyName, CounterpartyNo: f.CounterpartyNo,
		Amount: f.Amount, BalanceAfter: f.BalanceAfter,
		OccurredAt: f.OccurredAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toFeedEntryResponseList(list []*txdomain.FeedEntry) []FeedEntryResponse {
	out := make([]FeedEntryResponse, 0, len(list))
	for _, f := range list {
		out = append(out, toFeedEntryResponse(f))
	}
	return out
}

type TransferResponse struct {
	Id            string `json:"id"`
	TransactionNo string `json:"transactionNo"`
	Amount        int64  `json:"amount"`
	Status        string `json:"status"`
}

func toTransferResponse(t *txdomain.Transaction) TransferResponse {
	return TransferResponse{
		Id: t.Id, TransactionNo: t.TransactionNo, Amount: t.Amount, Status: string(t.Status),
	}
}
