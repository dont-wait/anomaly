package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
)

const (
	defaultDialTimeout = 10 * time.Second
	defaultSMTPTimeout = 30 * time.Second
)

// ErrSMTPNotConfigured SMTP chưa đủ cấu hình (thiếu Host).
var ErrSMTPNotConfigured = errors.New("smtp not configured")

// SMTPSender implement application MailSender bằng stdlib net/smtp.
// Gửi multipart/alternative (text thuần + HTML). Không log mail body
// (chứa OTP) hay password ở bất kỳ đâu.
type SMTPSender struct {
	cfg       *domain.SMTPConfig
	tlsConfig *tls.Config
}

// NewSMTPSender tạo sender; trả ErrSMTPNotConfigured khi thiếu Host.
func NewSMTPSender(cfg *domain.SMTPConfig) (*SMTPSender, error) {
	if cfg == nil || !cfg.Configured() {
		return nil, ErrSMTPNotConfigured
	}
	switch cfg.TLSMode {
	case "", "starttls", "implicit":
	default:
		return nil, fmt.Errorf("unsupported SMTP TLS mode %q", cfg.TLSMode)
	}
	return &SMTPSender{cfg: cfg, tlsConfig: &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}}, nil
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

func (s *SMTPSender) Send(ctx context.Context, msg maildomain.MailMessage) error {
	from := s.cfg.SenderAddress()
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))

	dialer := &net.Dialer{Timeout: defaultDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp connect: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return fmt.Errorf("smtp set deadline: %w", err)
		}
	} else {
		if err := conn.SetDeadline(time.Now().Add(defaultSMTPTimeout)); err != nil {
			return fmt.Errorf("smtp set deadline: %w", err)
		}
	}

	tlsConfig := s.tlsConfig.Clone()
	if s.cfg.TLSMode == "implicit" {
		secureConn := tls.Client(conn, tlsConfig)
		if err := secureConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("smtp tls: %w", err)
		}
		conn = secureConn
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if s.cfg.TLSMode != "implicit" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp server does not support required STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}

	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(buildMessage(from, msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}

	return client.Quit()
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
	b.WriteString(normalizeCRLF(msg.Text))
	b.WriteString("\r\n")

	b.WriteString("--")
	b.WriteString(boundary)
	b.WriteString("\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(normalizeCRLF(msg.HTML))
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

// normalizeCRLF chuẩn hóa line endings trong MIME body: bare LF → CRLF,
// existing CRLF giữ nguyên.
func normalizeCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}
