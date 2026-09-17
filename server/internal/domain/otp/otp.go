package otp

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const (
	OTPLength = 6
	TTL       = 60 * time.Second
	KeyPrefix = "otp:"
)

var maxCode = big.NewInt(1000000)

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ValidateEmail(email string) (string, error) {
	parsed, err := mail.ParseAddress(NormalizeEmail(email))
	if err != nil {
		return "", accountdomain.ErrInvalidEmail
	}
	return parsed.Address, nil
}

func ValidateCode(code string) error {
	if len(code) != OTPLength {
		return ErrOTPInvalid
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return ErrOTPInvalid
		}
	}
	return nil
}

func Key(email string) string {
	return KeyPrefix + NormalizeEmail(email)
}

func GenerateCode() (string, error) {
	n, err := rand.Int(rand.Reader, maxCode)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
