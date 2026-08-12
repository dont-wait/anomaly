package httpx

import "net/http"

func WriteError(w http.ResponseWriter, err error, status func(error) int) {
	code := status(err)
	if code == 0 {
		code = http.StatusInternalServerError
	}
	WriteJSON(w, code, map[string]string{"error": err.Error()})
}
