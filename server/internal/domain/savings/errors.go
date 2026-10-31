package savings

import "errors"

var (
	ErrInvalidDepositAmount = errors.New("savings deposit must be at least 1000000 VND")
	ErrUnsupportedTerm      = errors.New("unsupported savings term")
	ErrInvalidInterestRate  = errors.New("invalid savings annual interest rate")
	ErrInvalidOpenedAt      = errors.New("savings opening time is required")
	ErrInvalidMaturity      = errors.New("invalid savings maturity time")
	ErrAmountOverflow       = errors.New("savings amount exceeds int64 capacity")
	ErrInvalidContract      = errors.New("invalid savings contract")
	ErrNotMatured           = errors.New("savings contract is not yet mature")
	ErrAlreadySettled       = errors.New("savings contract already settled")
	ErrContractLocked       = errors.New("savings contract is locked")
	ErrInvalidSettlement    = errors.New("invalid savings settlement")
	ErrSettlementConflict   = errors.New("savings settlement revision or intent conflict")
)
