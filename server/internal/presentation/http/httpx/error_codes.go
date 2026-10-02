package httpx

type ErrorCode string

const (
	ErrorCodeBadRequest         ErrorCode = "BAD_REQUEST"
	ErrorCodeValidation         ErrorCode = "VALIDATION_ERROR"
	ErrorCodePayloadTooLarge    ErrorCode = "PAYLOAD_TOO_LARGE"
	ErrorCodeUnauthorized       ErrorCode = "UNAUTHORIZED"
	ErrorCodeForbidden          ErrorCode = "FORBIDDEN"
	ErrorCodeNotFound           ErrorCode = "NOT_FOUND"
	ErrorCodeConflict           ErrorCode = "CONFLICT"
	ErrorCodeGone               ErrorCode = "GONE"
	ErrorCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrorCodeInternal           ErrorCode = "INTERNAL_ERROR"

	ErrorCodeInvalidIdempotencyKey ErrorCode = "INVALID_IDEMPOTENCY_KEY"
	ErrorCodeIdempotencyConflict   ErrorCode = "IDEMPOTENCY_CONFLICT"
	ErrorCodeRegistrationPending   ErrorCode = "REGISTRATION_PENDING"
	ErrorCodeInvalidEmail          ErrorCode = "INVALID_EMAIL"
	ErrorCodeWeakPassword          ErrorCode = "WEAK_PASSWORD"
	ErrorCodeInvalidUsername       ErrorCode = "INVALID_USERNAME"
	ErrorCodeInvalidCCCD           ErrorCode = "INVALID_CCCD"
	ErrorCodeInvalidDate           ErrorCode = "INVALID_DATE"
	ErrorCodeUserAlreadyExists     ErrorCode = "USER_ALREADY_EXISTS"
	ErrorCodeInvalidCredentials    ErrorCode = "INVALID_CREDENTIALS"
	ErrorCodeInvalidVerifyPayload  ErrorCode = "INVALID_VERIFY_PAYLOAD"
	ErrorCodeAccountNotFound       ErrorCode = "ACCOUNT_NOT_FOUND"
	ErrorCodeInvalidAmount         ErrorCode = "INVALID_AMOUNT"
	ErrorCodeInsufficientFunds     ErrorCode = "INSUFFICIENT_FUNDS"

	ErrorCodeOTPExpired   ErrorCode = "OTP_EXPIRED"
	ErrorCodeOTPInvalid   ErrorCode = "INVALID_OTP"
	ErrorCodeMissingKey   ErrorCode = "MISSING_KEY"
	ErrorCodeFileTooLarge ErrorCode = "FILE_TOO_LARGE"
	ErrorCodeInvalidJSON  ErrorCode = "INVALID_JSON"
	ErrorCodeUserMismatch ErrorCode = "USER_MISMATCH"
)

func ErrorCodeForStatus(status int) ErrorCode {
	switch status {
	case 400:
		return ErrorCodeBadRequest
	case 401:
		return ErrorCodeUnauthorized
	case 403:
		return ErrorCodeForbidden
	case 404:
		return ErrorCodeNotFound
	case 409:
		return ErrorCodeConflict
	case 410:
		return ErrorCodeGone
	case 413:
		return ErrorCodePayloadTooLarge
	case 503:
		return ErrorCodeServiceUnavailable
	default:
		if status >= 500 {
			return ErrorCodeInternal
		}
		return ErrorCodeValidation
	}
}
