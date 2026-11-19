package notification

import "github.com/dont-wait/anomaly/internal/domain/notification"

// Thông báo chuyển khoản thành công cho người gửi và người nhận.
// Không đưa số dư hay số tài khoản đầy đủ vào: thông báo có thể hiện trên màn
// hình khóa. Xem specs/noti-transfer.md.
func init() {
	registerTransfer(notification.TypeTransferSent, "Chuyển tiền thành công", "-", "tới", "DEBIT")
	registerTransfer(notification.TypeTransferReceived, "Nhận tiền thành công", "+", "từ", "CREDIT")
}

func registerTransfer(t notification.Type, title, sign, preposition, direction string) {
	Register(Template{
		Type:     t,
		Category: notification.CategoryTransaction,
		Severity: notification.SeveritySuccess,
		Render: func(p map[string]string) (string, string, map[string]string, error) {
			if err := RequireParams(p, "transferId", "amount", "amountRaw", "counterpartyName", "last4"); err != nil {
				return "", "", nil, err
			}
			body := sign + p["amount"] + " VND " + preposition + " " + p["counterpartyName"] + " (•••• " + p["last4"] + ")."
			data := map[string]string{
				"transferId": p["transferId"],
				"direction":  direction,
				"amount":     p["amountRaw"],
				"currency":   "VND",
			}
			return title, body, data, nil
		},
	})
}
