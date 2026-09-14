package otp

import (
	"strings"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestGenerateCodeFormat(t *testing.T) {
	for i := 0; i < 1000; i++ {
		code, err := GenerateCode()
		if err != nil {
			t.Fatalf("GenerateCode() error = %v", err)
		}
		if len(code) != OTPLength {
			t.Fatalf("GenerateCode() = %q, want %d digits", code, OTPLength)
		}
		for _, c := range code {
			if c < '0' || c > '9' {
				t.Fatalf("GenerateCode() = %q, want digits only", code)
			}
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Alice@Example.COM "); got != "alice@example.com" {
		t.Fatalf("NormalizeEmail() = %q, want %q", got, "alice@example.com")
	}
}

func TestKey(t *testing.T) {
	if got := Key("Alice@Example.COM"); got != "otp:alice@example.com" {
		t.Fatalf("Key() = %q, want %q", got, "otp:alice@example.com")
	}
	if !strings.HasPrefix(Key("a@b.co"), KeyPrefix) {
		t.Fatalf("Key() = %q, want prefix %q", Key("a@b.co"), KeyPrefix)
	}
}

func TestValidateEmail(t *testing.T) {
	email, err := ValidateEmail(" Alice@Example.COM ")
	if err != nil {
		t.Fatalf("ValidateEmail() error = %v", err)
	}
	if email != "alice@example.com" {
		t.Fatalf("ValidateEmail() = %q, want %q", email, "alice@example.com")
	}

	if _, err := ValidateEmail("not-an-email"); err != accountdomain.ErrInvalidEmail {
		t.Fatalf("ValidateEmail() error = %v, want ErrInvalidEmail", err)
	}
	if _, err := ValidateEmail(""); err != accountdomain.ErrInvalidEmail {
		t.Fatalf("ValidateEmail() error = %v, want ErrInvalidEmail", err)
	}
}

func TestValidateCode(t *testing.T) {
	for _, code := range []string{"000000", "042817", "999999"} {
		if err := ValidateCode(code); err != nil {
			t.Fatalf("ValidateCode(%q) error = %v", code, err)
		}
	}
	for _, code := range []string{"", "12345", "1234567", "12345a", " 12345", "12-345"} {
		if err := ValidateCode(code); err != ErrOTPInvalid {
			t.Fatalf("ValidateCode(%q) error = %v, want ErrOTPInvalid", code, err)
		}
	}
}
