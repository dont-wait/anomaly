package account

import "errors"

var (
	ErrAccountNotFound   = errors.New("account not found")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrInsufficientFunds = errors.New("insufficient funds")
)
