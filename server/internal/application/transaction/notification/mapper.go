// Package notification maps committed transfer events to in-app notifications.
// Xem specs/noti-transfer.md.
package notification

import (
	"context"
	"errors"
	"strconv"
	"strings"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/notification"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

// errAccountNotReady: a transfer only targets in-system accounts, so a missing
// one means the account projection lags; cmd/notifier retries with backoff.
var errAccountNotReady = errors.New("transfer account is not in the projection yet")

type AccountLookup interface {
	FindByFinancialID(ctx context.Context, id string) (*accountdomain.UserAccount, error)
}

type CustomerLookup interface {
	FindByID(ctx context.Context, id string) (*accountdomain.Customer, error)
}

// Mapper turns TransferCompleted into TRANSFER_SENT (payer) and
// TRANSFER_RECEIVED (payee).
type Mapper struct {
	accounts  AccountLookup
	customers CustomerLookup
}

func NewMapper(accounts AccountLookup, customers CustomerLookup) *Mapper {
	return &Mapper{accounts: accounts, customers: customers}
}

func (m *Mapper) Supports(eventType string) bool { return eventType == journal.Completed }

func (m *Mapper) Map(ctx context.Context, record journal.Record) ([]notificationapp.Request, error) {
	if record.Event == nil {
		return nil, nil
	}
	event := record.Event
	source, err := m.accounts.FindByFinancialID(ctx, event.Payload.SourceAccountID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, errAccountNotReady
	}
	destination, err := m.accounts.FindByFinancialID(ctx, event.Payload.DestinationAccountID)
	if err != nil {
		return nil, err
	}
	if destination == nil {
		return nil, errAccountNotReady
	}
	sourceName, err := m.displayName(ctx, source)
	if err != nil {
		return nil, err
	}
	destinationName, err := m.displayName(ctx, destination)
	if err != nil {
		return nil, err
	}
	params := func(counterpartyName string, counterparty *accountdomain.UserAccount) map[string]string {
		return map[string]string{
			"transferId":       event.TransactionID,
			"amount":           formatVND(event.Payload.Amount),
			"amountRaw":        strconv.FormatInt(event.Payload.Amount, 10),
			"counterpartyName": counterpartyName,
			"last4":            last4(counterparty.AccountNo),
		}
	}
	return []notificationapp.Request{
		m.request(source, notification.TypeTransferSent, record, params(destinationName, destination)),
		m.request(destination, notification.TypeTransferReceived, record, params(sourceName, source)),
	}, nil
}

func (m *Mapper) request(owner *accountdomain.UserAccount, t notification.Type, record journal.Record, params map[string]string) notificationapp.Request {
	return notificationapp.Request{
		Audience:      notification.AudienceCustomer,
		RecipientID:   owner.Id,
		Type:          t,
		SourceEventID: record.ID,
		OccurredAt:    record.At,
		Params:        params,
	}
}

// displayName falls back to the masked account number so a missing profile
// never blocks the notification.
func (m *Mapper) displayName(ctx context.Context, account *accountdomain.UserAccount) (string, error) {
	customer, err := m.customers.FindByID(ctx, account.CustomerId)
	if err != nil {
		return "", err
	}
	if customer != nil {
		if name := strings.TrimSpace(customer.Profile.FullName); name != "" {
			return name, nil
		}
	}
	return "tài khoản •••• " + last4(account.AccountNo), nil
}

func last4(accountNo string) string {
	runes := []rune(accountNo)
	if len(runes) <= 4 {
		return accountNo
	}
	return string(runes[len(runes)-4:])
}

// formatVND groups thousands with dots: 1500000 -> "1.500.000".
func formatVND(amount int64) string {
	digits := strconv.FormatInt(amount, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var out []byte
	for i := range len(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, digits[i])
	}
	return sign + string(out)
}
