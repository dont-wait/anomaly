package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type RiskTransactionContext struct {
	TransactionID        string    `json:"transactionId"`
	SourceAccountNo      string    `json:"sourceAccountNo"`
	SourceAccountName    string    `json:"sourceAccountName"`
	DestinationAccountNo string    `json:"destinationAccountNo"`
	DestinationName      string    `json:"destinationName"`
	Amount               int64     `json:"amount"`
	Fee                  int64     `json:"fee"`
	Currency             string    `json:"currency"`
	Channel              string    `json:"channel"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"createdAt"`
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
		contexts[transfer.ID] = RiskTransactionContext{
			TransactionID:        transfer.ID,
			SourceAccountNo:      transfer.Source.AccountNo,
			SourceAccountName:    transfer.Source.Name,
			DestinationAccountNo: transfer.Destination.AccountNo,
			DestinationName:      transfer.Destination.Name,
			Amount:               transfer.Amount,
			Fee:                  transfer.Fee,
			Currency:             transfer.Currency,
			Channel:              transfer.Channel,
			Status:               transfer.Status,
			CreatedAt:            transfer.CreatedAt,
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return contexts, nil
}
