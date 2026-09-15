package mail

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
)

func TestBuildMessageMultipart(t *testing.T) {
	raw := string(buildMessage("no-reply@anomalybank.vn", maildomain.MailMessage{
		To:      "a@b.co",
		Subject: "[AnomalyBank] Mã xác thực OTP / Your OTP code",
		Text:    "plain 042817",
		HTML:    "<p>html 042817</p>",
	}))

	for _, want := range []string{
		"From: no-reply@anomalybank.vn",
		"To: a@b.co",
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative;",
		"Content-Type: text/plain;",
		"Content-Type: text/html;",
		"plain 042817",
		"<p>html 042817</p>",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("message missing %q", want)
		}
	}

	// Boundary khai báo phải khớp boundary dùng trong body.
	m := regexp.MustCompile(`boundary="([^"]+)"`).FindStringSubmatch(raw)
	if len(m) != 2 {
		t.Fatal("boundary not found in Content-Type")
	}
	if got := strings.Count(raw, "--"+m[1]); got != 3 {
		t.Fatalf("boundary markers = %d, want 3 (2 parts + closing)", got)
	}

	// Subject có dấu tiếng Việt phải mã hóa RFC2047, không để raw.
	if strings.Contains(raw, "Subject: [AnomalyBank] Mã") {
		t.Fatal("non-ASCII subject must be RFC2047-encoded")
	}
	encoded := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte("[AnomalyBank] Mã xác thực OTP / Your OTP code")) + "?="
	if !strings.Contains(raw, "Subject: "+encoded) {
		t.Fatal("RFC2047-encoded subject not found")
	}

	// CRLF chuẩn, không có LF trần.
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue // phần tử rỗng sau CRLF cuối cùng
		}
		if !strings.HasSuffix(line, "\r") {
			t.Fatalf("bare LF in line %q", line)
		}
	}
}

func TestEncodeSubjectASCIIPassthrough(t *testing.T) {
	if got := encodeSubject("plain ascii"); got != "plain ascii" {
		t.Fatalf("encodeSubject() = %q, want passthrough", got)
	}
}

func TestNewDisabledSenderFailsClosed(t *testing.T) {
	s := NewDisabledSender(nil)
	if err := s.Send(t.Context(), maildomain.MailMessage{}); err != ErrSMTPNotConfigured {
		t.Fatalf("Send() error = %v, want ErrSMTPNotConfigured", err)
	}
}
