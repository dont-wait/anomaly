package otp

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"time"

	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

//go:embed templates/otp_email.html
var otpEmailHTML string

var otpEmailTmpl = template.Must(template.New("otp_email").Parse(otpEmailHTML))

const otpEmailSubject = "[AnomalyBank] Mã xác thực OTP / Your OTP code"

type otpEmailData struct {
	Code          string
	ExpirySeconds int
	HeaderVI      string
	HeaderEN      string
	TitleVI       string
	IntroVI       string
	TitleEN       string
	IntroEN       string
}

// OTPEmailPurpose controls the copy rendered around the shared OTP email
// layout. The visual design remains the same for every OTP use case.
type OTPEmailPurpose string

const (
	OTPEmailAccount      OTPEmailPurpose = "account"
	OTPEmailTransfer     OTPEmailPurpose = "transfer"
	OTPEmailLoanContract OTPEmailPurpose = "loan_contract"
	OTPEmailChange       OTPEmailPurpose = "email_change"
)

type otpEmailCopy struct {
	headerVI string
	headerEN string
	titleVI  string
	introVI  string
	titleEN  string
	introEN  string
}

func otpEmailCopyFor(purpose OTPEmailPurpose) (otpEmailCopy, error) {
	switch purpose {
	case OTPEmailAccount:
		return otpEmailCopy{
			headerVI: "Xác thực tài khoản",
			headerEN: "Account verification",
			titleVI:  "Xác nhận tài khoản của bạn",
			introVI:  "Đây là mã xác thực (OTP) của bạn:",
			titleEN:  "Confirm your account",
			introEN:  "Your one-time password (OTP) is:",
		}, nil
	case OTPEmailChange:
		return otpEmailCopy{
			headerVI: "Đổi email",
			headerEN: "Email change",
			titleVI:  "Xác nhận đổi email",
			introVI:  "Đây là mã OTP để xác nhận dùng email này cho tài khoản. Nếu bạn không yêu cầu, hãy bỏ qua email này:",
			titleEN:  "Confirm your email change",
			introEN:  "Your OTP to confirm this email for your account. If this was not you, ignore this email:",
		}, nil
	case OTPEmailTransfer:
		return otpEmailCopy{
			headerVI: "Xác thực giao dịch",
			headerEN: "Transaction verification",
			titleVI:  "Xác nhận giao dịch chuyển tiền",
			introVI:  "Đây là mã OTP để xác nhận giao dịch chuyển tiền của bạn:",
			titleEN:  "Confirm your transfer",
			introEN:  "Your one-time password (OTP) to confirm this transfer is:",
		}, nil
	case OTPEmailLoanContract:
		return otpEmailCopy{
			headerVI: "Xác nhận hợp đồng vay", headerEN: "Loan contract confirmation",
			titleVI: "Xác nhận hợp đồng vay AnomalyBank", introVI: "Mã OTP xác nhận hợp đồng vay của bạn là:",
			titleEN: "Confirm your loan contract", introEN: "Your loan-contract confirmation code is:",
		}, nil
	default:
		return otpEmailCopy{}, fmt.Errorf("unsupported OTP email purpose %q", purpose)
	}
}

// renderOTPEmail render nội dung mail OTP song ngữ (VI + EN) từ code.
// ExpirySeconds lấy từ TTL của domain để không lệch với Redis.
func renderOTPEmail(code string) (subject, text, html string, err error) {
	return RenderOTPEmail(code, otpdomain.TTL, OTPEmailAccount)
}

// RenderOTPEmail renders the shared bilingual OTP email layout with the
// expiry and copy appropriate for the requested use case.
func RenderOTPEmail(code string, expiry time.Duration, purpose OTPEmailPurpose) (subject, text, html string, err error) {
	copy, err := otpEmailCopyFor(purpose)
	if err != nil {
		return "", "", "", err
	}

	expirySeconds := int(expiry.Seconds())
	data := otpEmailData{
		Code:          code,
		ExpirySeconds: expirySeconds,
		HeaderVI:      copy.headerVI,
		HeaderEN:      copy.headerEN,
		TitleVI:       copy.titleVI,
		IntroVI:       copy.introVI,
		TitleEN:       copy.titleEN,
		IntroEN:       copy.introEN,
	}

	var buf bytes.Buffer
	if err := otpEmailTmpl.Execute(&buf, data); err != nil {
		return "", "", "", err
	}

	text = fmt.Sprintf(`Xin chào,
%s %s
Mã có hiệu lực trong %d giây. Vì lý do bảo mật, vui lòng không chia sẻ mã này với bất kỳ ai.

---
Hello,
%s %s
This code expires in %d seconds. For your security, never share this code with anyone.

Email tự động từ AnomalyBank, vui lòng không trả lời.
This is an automated message from AnomalyBank. Please do not reply.`,
		data.IntroVI, code, expirySeconds, data.IntroEN, code, expirySeconds)

	return otpEmailSubject, text, buf.String(), nil
}
