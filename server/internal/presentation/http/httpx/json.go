package httpx

import (
	"encoding/json"
	"net/http"
)

type SuccessResponse[T any] struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type ErrorDetail struct {
	Code   ErrorCode `json:"code"`
	Field  string    `json:"field,omitempty"`
	Detail string    `json:"detail"`
}

type ErrorResponse struct {
	Status int           `json:"status"`
	Title  string        `json:"title"`
	Errors []ErrorDetail `json:"errors"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	buf, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

func WriteSuccess(w http.ResponseWriter, status int, message string, data any) {
	WriteJSON(w, status, SuccessResponse[any]{Status: status, Message: message, Data: data})
}
