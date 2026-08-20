package eventstore

import (
	"fmt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const (
	eventAccountCreated  = "AccountCreated"
	eventAccountWithdrew = "AccountWithdrew"
)

type accountCreatedPayload struct {
	Id       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type accountWithdrewPayload struct {
	Amount int64 `json:"amount"`
}

// streamName sinh tên stream cho 1 account cụ thể, dựa vào ID.
func streamName(accountID string) string {
	return fmt.Sprintf("account-%s", accountID)
}

// applyEvent áp 1 sự kiện đã đọc được lên 1 UserAccount, để dựng lại
// đúng trạng thái hiện tại từ toàn bộ lịch sử (replay).
func applyEvent(acc *accountdomain.UserAccount, eventType string, decode func(v any) error) error {
	switch eventType {
	case eventAccountCreated:
		var p accountCreatedPayload
		if err := decode(&p); err != nil {
			return err
		}
		acc.Id = p.Id
		acc.Username = p.Username
		acc.Email = p.Email
		acc.Amount = 0

	case eventAccountWithdrew:
		var p accountWithdrewPayload
		if err := decode(&p); err != nil {
			return err
		}
		acc.Amount -= p.Amount

	default:
		// bỏ qua sự kiện chưa biết, tránh lỗi khi mở rộng thêm sau
	}

	return nil
}
