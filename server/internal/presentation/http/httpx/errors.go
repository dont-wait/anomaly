package httpx

import (
	"net/http"

	"github.com/rs/zerolog"
)

func WriteError(w http.ResponseWriter, logger zerolog.Logger, err error, status func(error) int, codeResolvers ...func(error) ErrorCode) {
	statusCode := http.StatusInternalServerError
	if status != nil && err != nil {
		statusCode = status(err)
	}
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}

	errorCode := ErrorCodeForStatus(statusCode)
	if len(codeResolvers) > 0 && codeResolvers[0] != nil {
		errorCode = codeResolvers[0](err)
	}
	detail := http.StatusText(statusCode)
	if err != nil {
		detail = err.Error()
	}
	if statusCode >= http.StatusInternalServerError {
		if err != nil {
			logger.Error().Err(err).Msg("request failed")
		}
		errorCode = ErrorCodeInternal
		detail = "internal server error"
	}

	WriteJSON(w, statusCode, ErrorResponse{
		Status: statusCode,
		Title:  http.StatusText(statusCode),
		Errors: []ErrorDetail{{Code: errorCode, Detail: detail}},
	})
}
