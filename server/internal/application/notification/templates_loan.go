package notification

import (
	"fmt"

	"github.com/dont-wait/anomaly/internal/domain/notification"
)

func init() {
	Register(Template{
		Type:     notification.TypeLoanRepaymentDue,
		Category: notification.CategoryLoan,
		Severity: notification.SeverityWarning,
		Render: func(params map[string]string) (string, string, map[string]string, error) {
			if err := RequireParams(params, "kind", "loanId"); err != nil {
				return "", "", nil, err
			}
			var body string
			switch params["kind"] {
			case "T-5":
				body = "Một kỳ thanh toán khoản vay sắp đến hạn. Vui lòng kiểm tra lịch trả nợ trong ứng dụng."
			case "T-0":
				body = "Một kỳ thanh toán khoản vay đã đến hạn. Vui lòng kiểm tra và thanh toán trong ứng dụng."
			default:
				return "", "", nil, fmt.Errorf("unsupported loan reminder kind")
			}
			// No account number, amount or repayment date in push-visible text.
			return "Nhắc lịch thanh toán khoản vay", body, map[string]string{"loanId": params["loanId"]}, nil
		},
	})
}

// Giải ngân và trả nợ: chỉ số tiền và 4 số cuối tài khoản, không có số dư hay
// mã hợp đồng trong câu chữ (có thể hiện trên màn hình khóa). Xem specs/noti-loan.md.
func init() {
	Register(Template{
		Type:     notification.TypeLoanDisbursed,
		Category: notification.CategoryLoan,
		Severity: notification.SeveritySuccess,
		Render: func(p map[string]string) (string, string, map[string]string, error) {
			if err := RequireParams(p, "loanId", "amount", "amountRaw", "last4"); err != nil {
				return "", "", nil, err
			}
			body := "+" + p["amount"] + " VND vào tài khoản •••• " + p["last4"] + "."
			data := map[string]string{"loanId": p["loanId"], "amount": p["amountRaw"], "currency": "VND"}
			return "Giải ngân thành công", body, data, nil
		},
	})
	Register(Template{
		Type:     notification.TypeLoanInstallmentPaid,
		Category: notification.CategoryLoan,
		Severity: notification.SeveritySuccess,
		Render: func(p map[string]string) (string, string, map[string]string, error) {
			if err := RequireParams(p, "loanId", "amount", "amountRaw", "installmentNo", "last4"); err != nil {
				return "", "", nil, err
			}
			body := "-" + p["amount"] + " VND từ tài khoản •••• " + p["last4"] + " để thanh toán kỳ " + p["installmentNo"] + " của khoản vay."
			data := map[string]string{"loanId": p["loanId"], "amount": p["amountRaw"], "currency": "VND", "installmentNo": p["installmentNo"]}
			return "Thanh toán khoản vay thành công", body, data, nil
		},
	})
}
