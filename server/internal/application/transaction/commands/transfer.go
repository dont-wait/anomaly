package commands

import (
	"context"
	"fmt"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	txdomain "github.com/dont-wait/anomaly/internal/domain/transaction"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type TransferCommand struct {
	SourceAccountId string
	DestAccountNo   string
	Amount          int64
}

type TransferCommandHandler struct {
	accounts AccountRepository
	writer   TransactionWriter
}

func NewTransferCommandHandler(accounts AccountRepository, writer TransactionWriter) *TransferCommandHandler {
	return &TransferCommandHandler{accounts: accounts, writer: writer}
}

func (h *TransferCommandHandler) Handle(ctx context.Context, cmd TransferCommand) (*txdomain.Transaction, error) {
	if cmd.Amount <= 0 {
		return nil, txdomain.ErrInvalidAmount
	}

	source, err := h.accounts.FindByID(ctx, cmd.SourceAccountId)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, txdomain.ErrSourceAccountNotFound
	}

	dest, err := h.accounts.FindByAccountNo(ctx, cmd.DestAccountNo)
	if err != nil {
		return nil, err
	}
	if dest == nil {
		return nil, txdomain.ErrDestAccountNotFound
	}
	if source.Id == dest.Id {
		return nil, txdomain.ErrSameAccount
	}

	if err := source.Withdraw(cmd.Amount); err != nil {
		if err == accountdomain.ErrInsufficientFunds {
			return nil, txdomain.ErrInsufficientFunds
		}
		return nil, err
	}
	dest.Balance.Current += cmd.Amount
	dest.Version++
	dest.UpdatedAt = time.Now().UTC()

	now := time.Now().UTC()
	txID := bson.NewObjectID().Hex()
	tx := &txdomain.Transaction{
		Id:              txID,
		TransactionNo:   fmt.Sprintf("TXN-%s", txID),
		Type:            txdomain.TransactionTypeTransfer,
		SourceAccountId: source.Id,
		SourceAccountNo: source.AccountNo,
		DestAccountId:   dest.Id,
		DestAccountNo:   dest.AccountNo,
		Amount:          cmd.Amount,
		Currency:        string(source.Currency),
		Status:          txdomain.TransactionStatusPosted,
		CreatedAt:       now,
		PostedAt:        now,
	}

	sourceName := source.Username
	if source.Customer != nil && source.Customer.Profile.FullName != "" {
		sourceName = source.Customer.Profile.FullName
	}
	destName := dest.Username
	if dest.Customer != nil && dest.Customer.Profile.FullName != "" {
		destName = dest.Customer.Profile.FullName
	}

	feedEntries := []*txdomain.FeedEntry{
		{
			Id: bson.NewObjectID().Hex(), AccountId: source.Id, TransactionId: txID,
			Direction: txdomain.FeedDirectionOut, Type: txdomain.TransactionTypeTransfer,
			CounterpartyName: destName, CounterpartyNo: dest.AccountNo,
			Amount: cmd.Amount, BalanceAfter: source.Balance.Current, OccurredAt: now,
		},
		{
			Id: bson.NewObjectID().Hex(), AccountId: dest.Id, TransactionId: txID,
			Direction: txdomain.FeedDirectionIn, Type: txdomain.TransactionTypeTransfer,
			CounterpartyName: sourceName, CounterpartyNo: source.AccountNo,
			Amount: cmd.Amount, BalanceAfter: dest.Balance.Current, OccurredAt: now,
		},
	}

	if err := h.writer.Create(ctx, tx, feedEntries); err != nil {
		return nil, err
	}
	if err := h.accounts.Save(ctx, source); err != nil {
		return nil, err
	}
	if err := h.accounts.Save(ctx, dest); err != nil {
		return nil, err
	}

	return tx, nil
}
