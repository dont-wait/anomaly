package otp

import "errors"

var (
	ErrOTPExpired = errors.New("otp expired")
	ErrOTPInvalid = errors.New("invalid otp")
)
