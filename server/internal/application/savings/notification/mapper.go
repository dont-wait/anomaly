// Package notification maps committed savings money movements on the banking
// stream to in-app notifications. Xem specs/noti-savings.md.
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

// errAccountNotReady: the source account must exist, so a miss means the
// account projection lags; cmd/notifier retries with backoff.
var errAccountNotReady = errors.New("savings account is not in the projection yet")

type AccountLookup interface {
	FindByFinancialID(ctx context.Context, id string) (*accountdomain.UserAccount, error)
}

// Mapper handles SavingsOpened (deposit debited) and SavingsSettled (principal
// plus interest credited). Both carry the financial id of the source account.
type Mapper struct{ accounts AccountLookup }

func NewMapper(accounts AccountLookup) *Mapper { return &Mapper{accounts: accounts} }

func (m *Mapper) Supports(eventType string) bool {
	return eventType == journal.SavingsOpened || eventType == journal.SavingsSettled
}

func (m *Mapper) Map(ctx context.Context, record journal.Record) ([]notificationapp.Request, error) {
	if record.Savings == nil {
		return nil, nil
	}
	contract := record.Savings.Contract
	params := map[string]string{"savingsId": contract.ID}
	var t notification.Type
	switch record.Type {
	case journal.SavingsOpened:
		t = notification.TypeSavingsOpened
		params["amount"] = notificationapp.FormatVND(contract.Principal)
		params["amountRaw"] = strconv.FormatInt(contract.Principal, 10)
		params["termMonths"] = strconv.Itoa(contract.TermMonths)
	case journal.SavingsSettled:
		if contract.Settlement == nil {
			return nil, nil
		}
		t = notification.TypeSavingsMatured
		params["amount"] = notificationapp.FormatVND(contract.Settlement.TotalAmount)
		params["amountRaw"] = strconv.FormatInt(contract.Settlement.TotalAmount, 10)
		params["interest"] = notificationapp.FormatVND(contract.Settlement.InterestAmount)
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
	params["last4"] = notificationapp.Last4(owner.AccountNo)
	return []notificationapp.Request{{
		Audience:      notification.AudienceCustomer,
		RecipientID:   owner.Id,
		Type:          t,
		SourceEventID: record.ID,
		OccurredAt:    record.At,
		Params:        params,
	}}, nil
}
