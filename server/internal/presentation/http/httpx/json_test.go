package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestWriteSuccess(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteSuccess(rec, http.StatusOK, "loaded", map[string]string{"id": "1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var response SuccessResponse[map[string]string]
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != http.StatusOK {
		t.Fatalf("body status = %d, want 200", response.Status)
	}
	if response.Message != "loaded" || response.Data["id"] != "1" {
		t.Fatalf("response = %#v, want status, message and data", response)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteError(rec, zerolog.Nop(), errTest{}, func(error) int {
		return http.StatusBadRequest
	}, func(error) ErrorCode { return ErrorCodeInvalidEmail })

	var response ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != http.StatusBadRequest {
		t.Fatalf("body status = %d, want 400", response.Status)
	}
	if response.Title != "Bad Request" {
		t.Fatalf("title = %q, want Bad Request", response.Title)
	}
	if len(response.Errors) != 1 || response.Errors[0].Code != ErrorCodeInvalidEmail {
		t.Fatalf("errors = %#v, want invalid email", response.Errors)
	}
}

type errTest struct{}

func (errTest) Error() string { return "invalid email" }
