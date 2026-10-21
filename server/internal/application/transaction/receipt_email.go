package transaction

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
)

//go:embed templates/transfer_receipt.html
var transferReceiptHTML string

var transferReceiptTmpl = template.Must(template.New("transfer_receipt").Parse(transferReceiptHTML))

const transferReceiptSubject = "[AnomalyBank] Biên lai giao dịch / Transfer receipt"

// TransferReceiptData contains the canonical values needed to render a
// successful outgoing transfer receipt. Account numbers are masked in the
// email before rendering.
type TransferReceiptData struct {
	Reference        string
	SenderName       string
	SenderAccount    string
	RecipientName    string
	RecipientAccount string
	RecipientBank    string
	Amount           int64
	Fee              int64
	Currency         string
	Note             string
	PostedAt         time.Time
	AuthMethod       string
}

type transferReceiptView struct {
	Reference        string
	SenderName       string
	SenderAccount    string
	RecipientName    string
	RecipientAccount string
	RecipientBank    string
	Amount           string
	Fee              string
	Total            string
	Note             string
	PostedAt         string
	AuthMethod       string
}

// RenderTransferReceipt renders the bilingual HTML receipt and its plain-text
// fallback. It is intentionally independent of the mail transport so it can
// be tested without an SMTP server.
func RenderTransferReceipt(data TransferReceiptData) (subject, text, html string, err error) {
	currency := strings.TrimSpace(data.Currency)
	if currency == "" {
		currency = "VND"
	}
	authMethod := strings.TrimSpace(data.AuthMethod)
	if authMethod == "" {
		authMethod = "Email OTP"
	}
	postedAt := data.PostedAt
	if postedAt.IsZero() {
		postedAt = time.Now().UTC()
	}

	view := transferReceiptView{
		Reference:        data.Reference,
		SenderName:       data.SenderName,
		SenderAccount:    maskAccountNumber(data.SenderAccount),
		RecipientName:    data.RecipientName,
		RecipientAccount: maskAccountNumber(data.RecipientAccount),
		RecipientBank:    data.RecipientBank,
		Amount:           formatMoney(data.Amount, currency),
		Fee:              formatMoney(data.Fee, currency),
		Total:            formatMoney(data.Amount+data.Fee, currency),
		Note:             strings.TrimSpace(data.Note),
		PostedAt:         postedAt.In(timeutil.VietnamLocation).Format("02/01/2006 15:04:05") + " (UTC+07:00)",
		AuthMethod:       authMethod,
	}

	var buf bytes.Buffer
	if err := transferReceiptTmpl.Execute(&buf, view); err != nil {
		return "", "", "", err
	}

	text = fmt.Sprintf(`Xin chào %s,

Giao dịch chuyển tiền của bạn đã được thực hiện thành công.

Mã tham chiếu: %s
Thời gian: %s
Người chuyển: %s
Số tài khoản người chuyển: %s
Người nhận: %s
Số tài khoản người nhận: %s
Ngân hàng: %s
Số tiền: %s
Phí giao dịch: %s
Tổng cộng: %s
Nội dung: %s
Phương thức xác thực: %s

Hello %s,

Your transfer was completed successfully.

Reference: %s
Time: %s
Sender: %s
Sender account: %s
Recipient: %s
Recipient account: %s
Bank: %s
Amount: %s
Transaction fee: %s
Total: %s
Note: %s
Authentication: %s

Email tự động từ AnomalyBank, vui lòng không trả lời.
This is an automated message from AnomalyBank. Please do not reply.`,
		view.SenderName,
		view.Reference, view.PostedAt, view.SenderName, view.SenderAccount, view.RecipientName, view.RecipientAccount, view.RecipientBank,
		view.Amount, view.Fee, view.Total, valueOrDash(view.Note), view.AuthMethod,
		view.SenderName,
		view.Reference, view.PostedAt, view.SenderName, view.SenderAccount, view.RecipientName, view.RecipientAccount, view.RecipientBank,
		view.Amount, view.Fee, view.Total, valueOrDash(view.Note), view.AuthMethod)

	return transferReceiptSubject, text, buf.String(), nil
}

func formatMoney(amount int64, currency string) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	digits := strconv.FormatInt(amount, 10)
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "." + digits[i:]
	}
	return sign + digits + " " + currency
}

func maskAccountNumber(account string) string {
	account = strings.TrimSpace(account)
	if account == "" {
		return "—"
	}
	if len(account) <= 4 {
		return account
	}
	return strings.Repeat("•", len(account)-4) + account[len(account)-4:]
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}
