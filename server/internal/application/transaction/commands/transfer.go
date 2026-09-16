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
	//idempotencyKey (mã bất biến / mã chống trùng lặp) là 1 mã số duy nhất bạn tự tạo ra ở phía client, gắn vào 1 lần bấm nút, để server biết "đây có phải request cũ gọi lại hay không".
	IdempotencyKey string
}

// ví dụ IdempotencyKey
// khi bấm nút "Chuyển tiền 100k cho hiếu". App gửi request lên server, server xử lý xong (đã trừ tiền), nhưng đúng lúc đó mạng bị rớt — response chưa kịp quay về máy .
// App bạn thấy màn hình đứng im, tưởng bấm chưa thành công rồi bấm lại nút "Chuyển tiền" lần nữa.
// Nếu không có idempotencyKey: server nhận request thứ 2, coi nó là 1 lệnh chuyển tiền hoàn toàn mới rồi  trừ tiền lần 2 , người chuyển sẽ bị trừ tổng cộng 200k dù chỉ định chuyển 100k.
// Nếu có idempotencyKey: ngay khi  bấm nút lần đầu, app tự sinh ra 1 mã ngẫu nhiên, ví dụ "a1b2c3-...", gửi kèm lên server. Khi bấm lại lần 2 (do tưởng lỗi), app dùng lại đúng mã đó (không sinh mã mới) server nhận ra "à, mã này tôi xử lý rồi" không trừ tiền lần nữa, chỉ trả lại kết quả lần đầu.
type TransferCommandHandler struct {
	accounts AccountRepository
	writer   TransactionWriter
}

func NewTransferCommandHandler(accounts AccountRepository, writer TransactionWriter) *TransferCommandHandler {
	return &TransferCommandHandler{accounts: accounts, writer: writer}
}

const maxTransferRetries = 3

// Handle thực hiện chuyển tiền. Trước tiên kiểm tra IdempotencyKey đã được
// xử lý trước đó chưa (client gọi lại do timeout/mất mạng/bấm đúp) — nếu
// có rồi thì trả lại đúng kết quả cũ, KHÔNG trừ tiền lần nữa. Sau đó tự
// động thử lại tối đa maxTransferRetries lần nếu có request khác vừa sửa
// đổi 1 trong 2 account cùng lúc, hoặc vừa chèn cùng idempotency key.
func (h *TransferCommandHandler) Handle(ctx context.Context, cmd TransferCommand) (*txdomain.Transaction, error) {
	if cmd.Amount <= 0 {
		return nil, txdomain.ErrInvalidAmount
	}
	if cmd.IdempotencyKey == "" {
		return nil, txdomain.ErrIdempotencyKeyRequired
	}

	if existing, err := h.writer.FindBySourceAndIdempotencyKey(ctx, cmd.SourceAccountId, cmd.IdempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil // replay: request này đã xử lý trước đó
	}

	var lastErr error
	for attempt := 0; attempt < maxTransferRetries; attempt++ {
		tx, err := h.attempt(ctx, cmd)
		if err == nil {
			return tx, nil
		}
		if err == txdomain.ErrIdempotencyKeyConflict {
			// có request khác (cùng key) vừa chèn transaction trước mình —
			// đọc lại và trả kết quả của nó thay vì báo lỗi cho client
			existing, findErr := h.writer.FindBySourceAndIdempotencyKey(ctx, cmd.SourceAccountId, cmd.IdempotencyKey)
			if findErr != nil {
				return nil, findErr
			}
			if existing != nil {
				return existing, nil
			}
			lastErr = err
			continue
		}
		if err != accountdomain.ErrConcurrentModification {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func (h *TransferCommandHandler) attempt(ctx context.Context, cmd TransferCommand) (*txdomain.Transaction, error) {
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
		IdempotencyKey:  cmd.IdempotencyKey,
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

	// Nếu insert bị trùng idempotency_key (do 1 request khác cùng key vừa
	// chèn trước mình, race condition) — báo lên cho Handle() đọc lại kết
	// quả của người thắng, KHÔNG hoàn tác Save() bên dưới vì ta chưa chạy
	// tới đó.
	if err := h.writer.Create(ctx, tx, feedEntries); err != nil {
		if err == txdomain.ErrIdempotencyKeyConflict {
			return nil, err
		}
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
