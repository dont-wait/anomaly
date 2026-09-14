package mail

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/dont-wait/anomaly/internal/domain"
	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
)

// ErrSMTPNotConfigured SMTP chưa đủ cấu hình (thiếu Host).
var ErrSMTPNotConfigured = errors.New("smtp not configured")

// SMTPSender implement application MailSender bằng stdlib net/smtp.
// Gửi multipart/alternative (text thuần + HTML). Không log mail body
// (chứa OTP) hay password ở bất kỳ đâu.
type SMTPSender struct {
	cfg *domain.SMTPConfig
}

// NewSMTPSender tạo sender; trả ErrSMTPNotConfigured khi thiếu Host.
func NewSMTPSender(cfg *domain.SMTPConfig) (*SMTPSender, error) {
	if cfg == nil || !cfg.Configured() {
		return nil, ErrSMTPNotConfigured
	}
	return &SMTPSender{cfg: cfg}, nil
}

// DisabledSender là MailSender luôn fail-closed: mọi Send đều trả
// lỗi cấu hình. Dùng để API vẫn chạy (trả 500) khi SMTP chưa cấu hình.
type DisabledSender struct {
	err error
}

func NewDisabledSender(err error) *DisabledSender {
	if err == nil {
		err = ErrSMTPNotConfigured
	}
	return &DisabledSender{err: err}
}

func (s *DisabledSender) Send(_ context.Context, _ maildomain.MailMessage) error {
	return s.err
}

func (s *SMTPSender) Send(_ context.Context, msg maildomain.MailMessage) error {
	from := s.cfg.SenderAddress()
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	return smtp.SendMail(addr, auth, from, []string{msg.To}, buildMessage(from, msg))
}

// buildMessage dựng raw MIME multipart/alternative (text + HTML).
// Tách riêng thành hàm thuần để unit-test được mà không cần SMTP thật.
func buildMessage(from string, msg maildomain.MailMessage) []byte {
	boundary := newBoundary()

	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(msg.To)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(encodeSubject(msg.Subject))
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"")
	b.WriteString(boundary)
	b.WriteString("\"\r\n")
	b.WriteString("\r\n")

	b.WriteString("--")
	b.WriteString(boundary)
	b.WriteString("\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Text)
	b.WriteString("\r\n")

	b.WriteString("--")
	b.WriteString(boundary)
	b.WriteString("\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.HTML)
	b.WriteString("\r\n")

	b.WriteString("--")
	b.WriteString(boundary)
	b.WriteString("--\r\n")

	return []byte(b.String())
}

// newBoundary sinh boundary ngẫu nhiên để không trùng nội dung mail.
func newBoundary() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "anomaly-otp-boundary"
	}
	return "anomaly-" + hex.EncodeToString(buf[:])
}

// encodeSubject mã hóa subject theo RFC2047 khi có ký tự non-ASCII
// (tiếng Việt), giữ nguyên khi thuần ASCII.
func encodeSubject(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}
