package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
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

func TestNewSMTPSenderConfiguration(t *testing.T) {
	for _, cfg := range []*domain.SMTPConfig{nil, {}} {
		if _, err := NewSMTPSender(cfg); !errors.Is(err, ErrSMTPNotConfigured) {
			t.Fatalf("NewSMTPSender(%v) error = %v", cfg, err)
		}
	}
	if _, err := NewSMTPSender(&domain.SMTPConfig{Host: "localhost"}); err != nil {
		t.Fatalf("configured sender: %v", err)
	}
	customErr := errors.New("disabled for test")
	if err := NewDisabledSender(customErr).Send(t.Context(), maildomain.MailMessage{}); !errors.Is(err, customErr) {
		t.Fatalf("disabled sender error = %v", err)
	}
}

// A local SMTP peer exercises the actual network transaction without sending email.
func smtpPeer(t *testing.T, serve func(net.Conn) error) *domain.SMTPConfig {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = conn.Close() }()
		if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
			done <- err
			return
		}
		done <- serve(conn)
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		if err := <-done; err != nil {
			t.Errorf("SMTP peer: %v", err)
		}
	})
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	return &domain.SMTPConfig{Host: host, Port: portNumber, From: "sender@example.com"}
}

func smtpExchange(reader *bufio.Reader, conn net.Conn, prefix, reply string) error {
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, prefix) {
		return fmt.Errorf("command %q, want prefix %q", line, prefix)
	}
	_, err = io.WriteString(conn, reply+"\r\n")
	return err
}

func TestSMTPSenderSend(t *testing.T) {
	body := make(chan string, 1)
	cfg := smtpPeer(t, func(conn net.Conn) error {
		if _, err := io.WriteString(conn, "220 localhost ready\r\n"); err != nil {
			return err
		}
		reader := bufio.NewReader(conn)
		for _, step := range []struct{ command, reply string }{
			{"EHLO ", "250 localhost"},
			{"MAIL FROM:<sender@example.com>", "250 OK"},
			{"RCPT TO:<recipient@example.com>", "250 OK"},
			{"DATA\r\n", "354 Send message"},
		} {
			if err := smtpExchange(reader, conn, step.command, step.reply); err != nil {
				return err
			}
		}
		var message strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			if line == ".\r\n" {
				break
			}
			message.WriteString(line)
		}
		body <- message.String()
		if _, err := io.WriteString(conn, "250 Accepted\r\n"); err != nil {
			return err
		}
		return smtpExchange(reader, conn, "QUIT\r\n", "221 Bye")
	})
	sender, err := NewSMTPSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(t.Context(), maildomain.MailMessage{To: "recipient@example.com", Subject: "OTP", Text: "123456", HTML: "<p>123456</p>"}); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-body:
		for _, want := range []string{"Subject: OTP", "123456", "<p>123456</p>", "multipart/alternative"} {
			if !strings.Contains(message, want) {
				t.Errorf("message missing %q", want)
			}
		}
	default:
		t.Fatal("SMTP peer received no message")
	}
}

func TestSMTPSenderRecipientRejected(t *testing.T) {
	cfg := smtpPeer(t, func(conn net.Conn) error {
		if _, err := io.WriteString(conn, "220 localhost ready\r\n"); err != nil {
			return err
		}
		reader := bufio.NewReader(conn)
		for _, step := range []struct{ command, reply string }{
			{"EHLO ", "250 localhost"},
			{"MAIL FROM:", "250 OK"},
			{"RCPT TO:", "550 Recipient rejected"},
		} {
			if err := smtpExchange(reader, conn, step.command, step.reply); err != nil {
				return err
			}
		}
		// Rejection must close the connection without submitting DATA.
		line, err := reader.ReadString('\n')
		if err != io.EOF || line != "" {
			return fmt.Errorf("after rejection: command %q, error %v", line, err)
		}
		return nil
	})
	sender, err := NewSMTPSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(t.Context(), maildomain.MailMessage{To: "recipient@example.com"}); err == nil || !strings.Contains(err.Error(), "smtp rcpt to") {
		t.Fatalf("Send() error = %v, want recipient failure", err)
	}
}

func TestSMTPSenderDeadlineBoundsStalledGreeting(t *testing.T) {
	connected := make(chan struct{})
	cfg := smtpPeer(t, func(conn net.Conn) error {
		close(connected)
		_, err := io.Copy(io.Discard, conn)
		return err
	})
	sender, err := NewSMTPSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = sender.Send(ctx, maildomain.MailMessage{})
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("Send() error = %v, want timeout", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("request deadline did not bound SMTP operation")
	}
	select {
	case <-connected:
	default:
		t.Fatal("did not connect to stalled peer")
	}
}

func TestSMTPSenderCanceledBeforeDial(t *testing.T) {
	sender, err := NewSMTPSender(&domain.SMTPConfig{Host: "127.0.0.1", Port: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sender.Send(ctx, maildomain.MailMessage{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send() error = %v", err)
	}
}
