package httpx

import (
	"net/http"

	"github.com/rs/zerolog"
)

func WriteError(w http.ResponseWriter, logger zerolog.Logger, err error, status func(error) int) {
	code := http.StatusInternalServerError
	if status != nil && err != nil {
		code = status(err)
	}
	if code == 0 {
		code = http.StatusInternalServerError
	}

	if code >= http.StatusInternalServerError {
		if err != nil {
			logger.Error().Err(err).Msg("request failed")
		}
		WriteJSON(w, code, map[string]string{"error": "internal server error"})
		return
	}

	msg := http.StatusText(code)
	if err != nil {
		msg = err.Error()
	}
	WriteJSON(w, code, map[string]string{"error": msg})
}
