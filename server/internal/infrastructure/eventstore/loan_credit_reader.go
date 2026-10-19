package eventstore

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// CreditedLoan lưu thông tin khoản vay đã được
// cộng tiền thành công trong Banking Stream.
type CreditedLoan struct {
	LoanID     string
	AccountID  string
	Amount     int64
	CreditedAt time.Time
}

// ListCreditedLoans đọc các khoản giải ngân đã ghi
// bền vững trong Banking Stream.
//
// Đọc lại từ EventStoreDB giúp phát hiện khoản vay
// đã cộng tiền nhưng chưa hoàn tất LoanDisbursed,
// kể cả sau khi tiến trình bị tắt.
func (r *BankRepository) ListCreditedLoans(
	ctx context.Context,
) ([]CreditedLoan, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf(
			"bank eventstore client is not configured",
		)
	}

	state, err := r.load(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"load credited loans: %w",
			err,
		)
	}

	result := make(
		[]CreditedLoan,
		0,
		len(state.LoanCredits),
	)

	for loanID, credit := range state.LoanCredits {
		result = append(result, CreditedLoan{
			LoanID:     loanID,
			AccountID:  credit.AccountID,
			Amount:     credit.Amount,
			CreditedAt: credit.At,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LoanID < result[j].LoanID
	})

	return result, nil
}
