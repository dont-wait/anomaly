package loan

import "errors"

var (
	ErrInvalidAmount = errors.New(
		"requested loan amount must be greater than zero",
	)

	ErrUnsupportedTerm = errors.New(
		"unsupported loan term",
	)

	ErrInvalidLoanPackage = errors.New(
		"invalid loan package",
	)

	ErrInvalidDisbursementAccount = errors.New(
		"invalid disbursement account",
	)

	ErrInvalidApplicationStatus = errors.New(
		"invalid loan application status",
	)
)
