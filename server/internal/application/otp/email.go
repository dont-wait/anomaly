package otp

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"

	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

//go:embed templates/otp_email.html
var otpEmailHTML string

var otpEmailTmpl = template.Must(template.New("otp_email").Parse(otpEmailHTML))

const otpEmailSubject = "[AnomalyBank] Mã xác thực OTP / Your OTP code"

type otpEmailData struct {
	Code          string
	ExpirySeconds int
}

// renderOTPEmail render nội dung mail OTP song ngữ (VI + EN) từ code.
// ExpirySeconds lấy từ TTL của domain để không lệch với Redis.
func renderOTPEmail(code string) (subject, text, html string, err error) {
	data := otpEmailData{Code: code, ExpirySeconds: int(otpdomain.TTL.Seconds())}

	var buf bytes.Buffer
	if err := otpEmailTmpl.Execute(&buf, data); err != nil {
		return "", "", "", err
	}

	text = fmt.Sprintf(`Xin chào,
Đây là mã xác thực (OTP) AnomalyBank của bạn: %s
Mã có hiệu lực trong %d giây. Vì lý do bảo mật, vui lòng không chia sẻ mã này với bất kỳ ai.

---
Hello,
Your AnomalyBank one-time password (OTP) is: %s
This code expires in %d seconds. For your security, never share this code with anyone.

Email tự động từ AnomalyBank, vui lòng không trả lời.
This is an automated message from AnomalyBank. Please do not reply.`,
		code, int(otpdomain.TTL.Seconds()), code, int(otpdomain.TTL.Seconds()))

	return otpEmailSubject, text, buf.String(), nil
}
