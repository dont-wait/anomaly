package otp

import (
	"context"
	"errors"
	"time"
)

// ContractOTPInfo is the application-facing response, independent of Redis and SMTP.
type ContractOTPInfo struct {
	Channel           string    `json:"channel"`
	MaskedDestination string    `json:"maskedDestination"`
	ExpiresAt         time.Time `json:"expiresAt"`
	AttemptsLeft      int       `json:"attemptsLeft"`
	ResendAvailableAt time.Time `json:"resendAvailableAt"`
}

var (
	ErrExpired  = errors.New("otp expired")
	ErrInvalid  = errors.New("invalid otp")
	ErrExceeded = errors.New("otp attempts exceeded")
	ErrTooSoon  = errors.New("otp resend too soon")
)

// ContractOTPService is a port provided by an infrastructure adapter.
type ContractOTPService interface {
	Issue(context.Context, string, string) (ContractOTPInfo, error)
	Verify(context.Context, string, string) (int, error)
	Consume(context.Context, string) error
}
