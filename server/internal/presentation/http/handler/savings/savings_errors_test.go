package savings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	savingscommands "github.com/dont-wait/anomaly/internal/application/savings/commands"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	savingsdomain "github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

func TestSavingsCommandErrorResponses(t *testing.T) {
	cases := []struct {
		name, code string
		err        error
		status     int
		wantLog    bool
	}{
		{name: "not settled", err: savingscommands.ErrSavingsNotSettled, status: http.StatusConflict, code: "SAVINGS_NOT_SETTLED"},
		{name: "confirmation required", err: savingscommands.ErrConfirmationRequired, status: http.StatusBadRequest, code: "CONFIRMATION_REQUIRED"},
		{name: "invalid idempotency key", err: savingscommands.ErrInvalidKey, status: http.StatusBadRequest, code: "INVALID_IDEMPOTENCY_KEY"},
		{name: "unsupported withdrawal type", err: savingscommands.ErrUnsupportedWithdrawalType, status: http.StatusBadRequest, code: "UNSUPPORTED_WITHDRAWAL_TYPE"},
		{name: "invalid withdrawal quote", err: savingscommands.ErrInvalidWithdrawalQuote, status: http.StatusBadRequest, code: "INVALID_QUOTE"},
		{name: "missing withdrawal quote", err: savingsqueries.ErrQuoteNotFound, status: http.StatusNotFound, code: "QUOTE_NOT_FOUND"},
		{name: "expired withdrawal quote", err: savingscommands.ErrWithdrawalQuoteExpired, status: http.StatusGone, code: "QUOTE_EXPIRED"},
		{name: "not owned", err: savingscommands.ErrSavingsNotFound, status: http.StatusNotFound, code: "SAVINGS_NOT_FOUND"},
		{name: "already settled", err: savingsdomain.ErrAlreadySettled, status: http.StatusConflict, code: "SAVINGS_ALREADY_SETTLED"},
		{name: "not matured", err: savingsdomain.ErrNotMatured, status: http.StatusUnprocessableEntity, code: "SAVINGS_NOT_MATURED"},
		{name: "locked", err: savingsdomain.ErrContractLocked, status: http.StatusLocked, code: "SAVINGS_LOCKED"},
		{name: "payment account unavailable", err: savingscommands.ErrDestinationUnavailable, status: http.StatusUnprocessableEntity, code: "DESTINATION_ACCOUNT_UNAVAILABLE"},
		{name: "settlement unconfirmed", err: savingscommands.ErrSavingsSettlementUnconfirmed, status: http.StatusServiceUnavailable, code: "SETTLEMENT_UNAVAILABLE", wantLog: true},
		{name: "writes disabled", err: savingscommands.ErrSavingsWritesDisabled, status: http.StatusServiceUnavailable, code: "SAVINGS_WRITES_DISABLED", wantLog: true},
		{name: "idempotency conflict", err: savingscommands.ErrIdempotencyConflict, status: http.StatusConflict, code: "IDEMPOTENCY_CONFLICT"},
		{name: "invalid source", err: savingsqueries.ErrInvalidSource, status: http.StatusUnprocessableEntity, code: "INVALID_SOURCE_ACCOUNT"},
		{name: "dependency failure", err: errors.New("sensitive database connection detail"), status: http.StatusServiceUnavailable, code: "SAVINGS_COMMAND_UNAVAILABLE", wantLog: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logBuffer, nil)))
			defer slog.SetDefault(old)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/savings/contracts", nil)
			(&Handler{}).fail(w, r, "test_operation", tt.err)
			if w.Code != tt.status {
				t.Fatalf("HTTP status=%d, want=%d: %s", w.Code, tt.status, w.Body.String())
			}
			var result apiError
			decoder := json.NewDecoder(w.Body)
			if err := decoder.Decode(&result); err != nil || result.Code != tt.code {
				t.Fatalf("error=%+v, decode=%v, want code=%s", result, err, tt.code)
			}
			var second apiError
			if err := decoder.Decode(&second); err != io.EOF {
				t.Fatalf("multiple HTTP responses, second decode=%v", err)
			}
			if strings.Contains(w.Body.String(), "sensitive database") {
				t.Fatal("backend error leaked in HTTP response")
			}
			if tt.wantLog && (!strings.Contains(logBuffer.String(), "test_operation") || !strings.Contains(logBuffer.String(), "savings")) {
				t.Fatalf("missing error operation context: %q", logBuffer.String())
			}
			if !tt.wantLog && logBuffer.Len() > 0 {
				t.Fatalf("expected business failure to avoid error logging, got %q", logBuffer.String())
			}
		})
	}
}

func TestSavingsHandlerDisabledWhenCatalogInitializationFails(t *testing.T) {
	// A failed EnsureSeeded at startup explicitly nils Service and Commands.
	// Non-savings handlers are kept by the parent router; savings fails closed.
	mux := http.NewServeMux()
	reg := openapi.NewRegistry(mux, false)
	RegisterRoutes(reg, &Handler{}, savingsTokenStub{})
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/savings/packages", ""},
		{http.MethodPost, "/api/savings/quotes", `{}`},
		{http.MethodPost, "/api/savings/contracts", `{}`},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer valid")
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s: status %d (body %s), want 503", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
