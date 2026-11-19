// Package notification maps committed loan money movements on the banking
// stream to in-app notifications. Xem specs/noti-loan.md.
package notification

import (
	"context"
	"errors"
	"strconv"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/notification"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

// errAccountNotReady: the credited/debited account must exist, so a miss means
// the account projection lags; cmd/notifier retries with backoff.
var errAccountNotReady = errors.New("loan account is not in the projection yet")

type AccountLookup interface {
	FindByFinancialID(ctx context.Context, id string) (*accountdomain.UserAccount, error)
}

// Mapper handles LoanDisbursementCredited and LoanInstallmentPaid. Both carry
// the financial account id of the customer's own account.
type Mapper struct{ accounts AccountLookup }

func NewMapper(accounts AccountLookup) *Mapper { return &Mapper{accounts: accounts} }

func (m *Mapper) Supports(eventType string) bool {
	return eventType == journal.LoanDisbursementCredited || eventType == journal.LoanInstallmentPaid
}

func (m *Mapper) Map(ctx context.Context, record journal.Record) ([]notificationapp.Request, error) {
	var (
		t      notification.Type
		amount int64
		extra  map[string]string
	)
	switch record.Type {
	case journal.LoanDisbursementCredited:
		t, amount = notification.TypeLoanDisbursed, record.LoanAmount
	case journal.LoanInstallmentPaid:
		if record.LoanPayment == nil {
			return nil, nil
		}
		t, amount = notification.TypeLoanInstallmentPaid, record.LoanPayment.Amount
		extra = map[string]string{"installmentNo": strconv.Itoa(record.LoanPayment.InstallmentNo)}
	default:
		return nil, nil
	}
	owner, err := m.accounts.FindByFinancialID(ctx, record.AccountID)
	if err != nil {
		return nil, err
	}
	if owner == nil {
		return nil, errAccountNotReady
	}
	params := map[string]string{
		"loanId":    record.LoanID,
		"amount":    notificationapp.FormatVND(amount),
		"amountRaw": strconv.FormatInt(amount, 10),
		"last4":     notificationapp.Last4(owner.AccountNo),
	}
	for k, v := range extra {
		params[k] = v
	}
	return []notificationapp.Request{{
		Audience:      notification.AudienceCustomer,
		RecipientID:   owner.Id,
		Type:          t,
		SourceEventID: record.ID,
		OccurredAt:    record.At,
		Params:        params,
	}}, nil
}
