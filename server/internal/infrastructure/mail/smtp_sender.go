package mail

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/dont-wait/anomaly/internal/domain"
)

var ErrSMTPNotConfigured = errors.New("smtp not configured")

// SMTPSender implement application MailSender bằng stdlib net/smtp.
// Không log mail body (chứa OTP) hay password ở bất kỳ đâu.
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

func (s *DisabledSender) Send(_ context.Context, _, _, _ string) error {
	return s.err
}

func (s *SMTPSender) Send(_ context.Context, to, subject, body string) error {
	from := s.cfg.SenderAddress()
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	var msg strings.Builder
	msg.WriteString("From: ")
	msg.WriteString(from)
	msg.WriteString("\r\n")
	msg.WriteString("To: ")
	msg.WriteString(to)
	msg.WriteString("\r\n")
	msg.WriteString("Subject: ")
	msg.WriteString(subject)
	msg.WriteString("\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg.String()))
}
