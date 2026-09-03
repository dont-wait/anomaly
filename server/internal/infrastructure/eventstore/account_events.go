package eventstore

import (
	"fmt"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const (
	EventAccountCreated  = "AccountCreated"
	EventAccountVerified = "AccountVerified"
	EventAccountWithdraw = "AccountWithdraw"
)

type accountCreatedPayload struct {
	Account accountdomain.UserAccount `json:"account"`
}

type accountVerifiedPayload struct {
	Session              accountdomain.KYCSession `json:"session"`
	VerifiedKYCSessionId string                   `json:"verifiedKycSessionId"`
	Version              int64                    `json:"version"`
	UpdatedAt            time.Time                `json:"updatedAt"`
}

type accountWithdrawPayload struct {
	Amount    int64     `json:"amount"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// streamName sinh tên stream cho 1 account cụ thể, dựa vào ID.
func streamName(accountID string) string {
	return fmt.Sprintf("account-%s", accountID)
}

// applyEvent áp 1 sự kiện đã đọc được lên 1 UserAccount, để dựng lại
// đúng trạng thái hiện tại từ toàn bộ lịch sử (replay).
func applyEvent(
	acc *accountdomain.UserAccount,
	eventType string,
	decode func(v any) error,
) error {
	switch eventType {
	case EventAccountCreated:
		var p accountCreatedPayload
		if err := decode(&p); err != nil {
			return err
		}
		*acc = p.Account

	case EventAccountVerified:
		var p accountVerifiedPayload
		if err := decode(&p); err != nil {
			return err
		}
		if acc.Customer == nil {
			return fmt.Errorf("account %s has no customer", acc.Id)
		}
		acc.KYCSessions = append(acc.KYCSessions, &p.Session)
		acc.Customer.VerifiedKYCSessionId = p.VerifiedKYCSessionId
		acc.Customer.KYCStatus = accountdomain.KYCStatusVerified
		acc.Customer.UpdatedAt = p.UpdatedAt
		acc.Version = p.Version
		acc.UpdatedAt = p.UpdatedAt

	case EventAccountWithdraw:
		var p accountWithdrawPayload
		if err := decode(&p); err != nil {
			return err
		}
		acc.Balance.Current -= p.Amount
		acc.Version = p.Version
		acc.UpdatedAt = p.UpdatedAt
	}

	return nil
}
