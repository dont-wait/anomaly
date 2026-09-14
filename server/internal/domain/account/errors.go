package account

import "errors"

var (
	ErrInvalidIdempotencyKey = errors.New("idempotency key must be a UUID")
	ErrIdempotencyConflict   = errors.New("idempotency key already used with different registration data")
	ErrRegistrationPending   = errors.New("registration is still being persisted; retry shortly")
	ErrInvalidEmail          = errors.New("invalid email")
	ErrWeakPassword          = errors.New("password must be at least 8 characters")
	ErrInvalidUsername       = errors.New("username must not be empty")
	ErrInvalidCCCD           = errors.New("CCCD must be exactly 12 digits")
	ErrInvalidDate           = errors.New("invalid or missing date")
	ErrUserAlreadyExists     = errors.New("user already exists")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrInvalidVerifyPayload  = errors.New("all identity URLs are required")
	ErrAccountNotFound       = errors.New("account not found")
	ErrInvalidAmount         = errors.New("invalid amount")
	ErrInsufficientFunds     = errors.New("insufficient funds")
)
