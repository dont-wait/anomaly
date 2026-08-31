package account

import "errors"

var (
	ErrInvalidEmail       = errors.New("invalid email")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrInvalidUsername    = errors.New("username must not be empty")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountNotFound    = errors.New("account not found")
	ErrInvalidAmount      = errors.New("invalid amount")
	ErrInsufficientFunds  = errors.New("insufficient funds")
)
