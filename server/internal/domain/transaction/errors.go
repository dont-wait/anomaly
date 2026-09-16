package transaction

import "errors"

var (
	ErrInvalidAmount         = errors.New("invalid amount")
	ErrSameAccount           = errors.New("source and destination account must differ")
	ErrSourceAccountNotFound = errors.New("source account not found")
	ErrDestAccountNotFound   = errors.New("destination account not found")
	ErrInsufficientFunds     = errors.New("insufficient funds")
)
