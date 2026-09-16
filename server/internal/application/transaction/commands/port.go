package commands

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	txdomain "github.com/dont-wait/anomaly/internal/domain/transaction"
)

type AccountRepository interface {
	FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error)
	FindByAccountNo(ctx context.Context, accountNo string) (*accountdomain.UserAccount, error)
	Save(ctx context.Context, a *accountdomain.UserAccount) error
}

// TransactionWriter ghi transaction gốc + các feed entry liên quan.
type TransactionWriter interface {
	Create(ctx context.Context, tx *txdomain.Transaction, feedEntries []*txdomain.FeedEntry) error
	// FindBySourceAndIdempotencyKey dùng để kiểm tra 1 request có phải là
	// "gọi lại" của 1 request trước đó hay không (retry do mất mạng, bấm
	// đúp nút...) — nếu tìm thấy, trả về đúng kết quả cũ thay vì xử lý lại.
	FindBySourceAndIdempotencyKey(ctx context.Context, sourceAccountId, idempotencyKey string) (*txdomain.Transaction, error)
}
