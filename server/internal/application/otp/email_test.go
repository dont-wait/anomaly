package otp

import (
	"strings"
	"testing"
)

func TestRenderOTPEmailBilingualAndBranded(t *testing.T) {
	subject, text, html, err := renderOTPEmail("042817")
	if err != nil {
		t.Fatalf("renderOTPEmail() error = %v", err)
	}

	if strings.Contains(subject, "042817") {
		t.Fatalf("subject %q must not contain the code", subject)
	}

	// HTML: mã xuất hiện ở cả 2 khối VI + EN.
	if got := strings.Count(html, "042817"); got != 2 {
		t.Fatalf("code occurrences in HTML = %d, want 2", got)
	}
	if strings.Contains(html, "{{") {
		t.Fatal("HTML contains unrendered template action")
	}

	// Song ngữ.
	for _, s := range []string{
		"Xác nhận tài khoản của bạn", "không chia sẻ mã này",
		"Confirm your account", "never share this code",
	} {
		if !strings.Contains(html, s) {
			t.Fatalf("HTML missing %q", s)
		}
	}

	// Màu thương hiệu từ client/src/index.css.
	for _, c := range []string{"#00236f", "#b582b5", "#8b5cf6", "#faf8ff", "#f2f3ff", "#ffdad6"} {
		if !strings.Contains(html, c) {
			t.Fatalf("HTML missing brand color %q", c)
		}
	}

	// Bản text fallback cũng song ngữ và chứa mã + TTL từ domain.
	for _, s := range []string{"042817", "60 giây", "60 seconds", "không chia sẻ", "never share"} {
		if !strings.Contains(text, s) {
			t.Fatalf("text missing %q", s)
		}
	}
}
