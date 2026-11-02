package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type RiskTransactionContext struct {
	TransactionID            string     `json:"transactionId"`
	SourceAccountID          string     `json:"sourceAccountId"`
	SourceAccountStatus      string     `json:"sourceAccountStatus"`
	SourceAccountNo          string     `json:"sourceAccountNo"`
	SourceAccountName        string     `json:"sourceAccountName"`
	DestinationAccountID     string     `json:"destinationAccountId"`
	DestinationAccountStatus string     `json:"destinationAccountStatus"`
	DestinationAccountNo     string     `json:"destinationAccountNo"`
	DestinationName          string     `json:"destinationName"`
	Amount                   int64      `json:"amount"`
	Fee                      int64      `json:"fee"`
	Currency                 string     `json:"currency"`
	Channel                  string     `json:"channel"`
	Status                   string     `json:"status"`
	CreatedAt                time.Time  `json:"createdAt"`
	OTPExpiresAt             *time.Time `json:"otpExpiresAt,omitempty"`
	OTPVerifiedAt            *time.Time `json:"otpVerifiedAt,omitempty"`
	CompletedAt              *time.Time `json:"completedAt,omitempty"`
	FailedAt                 *time.Time `json:"failedAt,omitempty"`
	CancelledAt              *time.Time `json:"cancelledAt,omitempty"`
}

// FindRiskTransactionContexts reads the projection needed by the admin risk
// monitor without applying customer ownership checks.
func (r *TransferRepository) FindRiskTransactionContexts(ctx context.Context, transactionIDs []string) (map[string]RiskTransactionContext, error) {
	contexts := make(map[string]RiskTransactionContext, len(transactionIDs))
	if len(transactionIDs) == 0 {
		return contexts, nil
	}
	cursor, err := r.db.Collection("transactions").Find(ctx, bson.M{"transaction_id": bson.M{"$in": transactionIDs}})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var transfer TransferDocument
		if err := cursor.Decode(&transfer); err != nil {
			return nil, err
		}
		status := transfer.Status
		if status == "awaiting_otp" && !transfer.OTPExpiresAt.IsZero() && time.Now().UTC().After(transfer.OTPExpiresAt) {
			status = "expired"
		}
		contexts[transfer.ID] = RiskTransactionContext{
			TransactionID:        transfer.ID,
			SourceAccountID:      transfer.Source.AccountID.Hex(),
			SourceAccountNo:      transfer.Source.AccountNo,
			SourceAccountName:    transfer.Source.Name,
			DestinationAccountID: transfer.Destination.AccountID.Hex(),
			DestinationAccountNo: transfer.Destination.AccountNo,
			DestinationName:      transfer.Destination.Name,
			Amount:               transfer.Amount,
			Fee:                  transfer.Fee,
			Currency:             transfer.Currency,
			Channel:              transfer.Channel,
			Status:               status,
			CreatedAt:            transfer.CreatedAt,
			OTPExpiresAt:         timePointer(transfer.OTPExpiresAt),
			OTPVerifiedAt:        transfer.OTPVerifiedAt,
			CompletedAt:          transfer.CompletedAt,
			FailedAt:             transfer.FailedAt,
			CancelledAt:          transfer.CancelledAt,
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	accountNos := make([]string, 0, len(contexts)*2)
	for _, item := range contexts {
		accountNos = append(accountNos, item.SourceAccountNo, item.DestinationAccountNo)
	}
	if len(accountNos) == 0 {
		return contexts, nil
	}
	accounts, err := r.db.Collection("accounts").Find(ctx,
		bson.M{"account_no": bson.M{"$in": accountNos}},
		options.Find().SetProjection(bson.M{"account_no": 1, "status": 1, "_id": 0}),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = accounts.Close(ctx) }()
	statuses := make(map[string]string)
	for accounts.Next(ctx) {
		var item struct {
			AccountNo string `bson:"account_no"`
			Status    string `bson:"status"`
		}
		if err := accounts.Decode(&item); err != nil {
			return nil, err
		}
		statuses[item.AccountNo] = item.Status
	}
	if err := accounts.Err(); err != nil {
		return nil, err
	}
	for id, item := range contexts {
		item.SourceAccountStatus = statuses[item.SourceAccountNo]
		item.DestinationAccountStatus = statuses[item.DestinationAccountNo]
		contexts[id] = item
	}
	return contexts, nil
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
