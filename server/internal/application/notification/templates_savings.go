package notification

import "github.com/dont-wait/anomaly/internal/domain/notification"

// Gửi và tất toán tiết kiệm. Chỉ số tiền, kỳ hạn và 4 số cuối tài khoản; không
// có số dư hay mã sổ trong câu chữ (có thể hiện trên màn hình khóa).
// Xem specs/noti-savings.md.
func init() {
	Register(Template{
		Type:     notification.TypeSavingsOpened,
		Category: notification.CategorySavings,
		Severity: notification.SeveritySuccess,
		Render: func(p map[string]string) (string, string, map[string]string, error) {
			if err := RequireParams(p, "savingsId", "amount", "amountRaw", "termMonths", "last4"); err != nil {
				return "", "", nil, err
			}
			body := "-" + p["amount"] + " VND từ tài khoản •••• " + p["last4"] + " gửi tiết kiệm kỳ hạn " + p["termMonths"] + " tháng."
			data := map[string]string{"savingsId": p["savingsId"], "amount": p["amountRaw"], "currency": "VND", "termMonths": p["termMonths"]}
			return "Gửi tiết kiệm thành công", body, data, nil
		},
	})
	Register(Template{
		Type:     notification.TypeSavingsMatured,
		Category: notification.CategorySavings,
		Severity: notification.SeveritySuccess,
		Render: func(p map[string]string) (string, string, map[string]string, error) {
			if err := RequireParams(p, "savingsId", "amount", "amountRaw", "interest", "last4"); err != nil {
				return "", "", nil, err
			}
			body := "+" + p["amount"] + " VND (gồm lãi " + p["interest"] + " VND) vào tài khoản •••• " + p["last4"] + "."
			data := map[string]string{"savingsId": p["savingsId"], "amount": p["amountRaw"], "currency": "VND"}
			return "Tất toán tiết kiệm thành công", body, data, nil
		},
	})
}
