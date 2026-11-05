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
				body = "Một kỳ thanh toán khoản vay sẽ đến hạn sau 5 ngày. Vui lòng kiểm tra lịch trả nợ trong ứng dụng."
			case "T-0":
				body = "Hôm nay là ngày đến hạn một kỳ thanh toán khoản vay. Vui lòng kiểm tra và thanh toán đúng hạn."
			default:
				return "", "", nil, fmt.Errorf("unsupported loan reminder kind")
			}
			// No account number, amount or repayment date in push-visible text.
			return "Nhắc lịch thanh toán khoản vay", body, map[string]string{"loanId": params["loanId"]}, nil
		},
	})
}
