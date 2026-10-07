package transaction

import (
	"net/http"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/infrastructure/otp"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

// These documentation DTOs describe the existing direct JSON responses.
type recipientResponse struct {
	AccountNo string `json:"accountNo"`
	Name      string `json:"name"`
	BankCode  string `json:"bankCode"`
}

type transactionResponseDoc struct {
	ID           string `json:"id"`
	Reference    string `json:"reference"`
	Direction    string `json:"direction"`
	Status       string `json:"status"`
	Kind         string `json:"kind"`
	Amount       int64  `json:"amount"`
	Fee          int64  `json:"fee"`
	Note         string `json:"note"`
	Counterparty struct {
		Name      string `json:"name"`
		AccountNo string `json:"accountNo"`
		BankCode  string `json:"bankCode"`
		Bank      string `json:"bank,omitempty"`
	} `json:"counterparty"`
	CreatedAt    time.Time `json:"createdAt"`
	BalanceAfter *int64    `json:"balanceAfter,omitempty"`
}

type transferErrorDoc struct {
	Error        string `json:"error"`
	Code         string `json:"code"`
	AttemptsLeft *int   `json:"attemptsLeft,omitempty"`
}

func RegisterRoutes(router *openapi.Registry, h *Handler, tokenSvc queries.TokenService) {
	auth := func(pattern string, fn http.HandlerFunc, op openapi.Operation) {
		op.Auth = true
		op.Tags = []string{"Transactions"}
		op.Description += " Responses are direct JSON without a data wrapper. See server/docs/transactions-api.md for examples and retry rules."
		op.FailureStatuses = append(op.FailureStatuses, http.StatusUnauthorized, http.StatusInternalServerError)
		op.FailureResponses = make(map[int]any)
		for _, status := range op.FailureStatuses {
			if status != http.StatusUnauthorized {
				op.FailureResponses[status] = transferErrorDoc{}
			}
		}
		secured := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			fn.ServeHTTP(w, r)
		})
		router.Handle(pattern, middleware.RequireAuth(tokenSvc)(secured), op)
	}
	query := func(name, example string, required bool) openapi.Parameter {
		return openapi.Parameter{Name: name, In: "query", Type: "string", Example: example, Required: required}
	}
	limit := func(example string) openapi.Parameter {
		return openapi.Parameter{Name: "limit", In: "query", Type: "integer", Example: example}
	}
	auth("GET /api/accounts/lookup", h.Lookup, openapi.Operation{
		ID: "lookupTransferRecipient", Summary: "Look up an internal transfer recipient",
		Description: "bankCode must be ANOMALY. accountNo must contain 6–19 digits. Self-transfer is rejected.",
		Parameters:  []openapi.Parameter{query("bankCode", "ANOMALY", true), query("accountNo", "12345678901234", true)},
		Response:    recipientResponse{}, FailureStatuses: []int{400, 404, 422},
	})
	auth("POST /api/transfers", h.CreateTransfer, openapi.Operation{
		ID: "createTransfer", Summary: "Create a transfer and send confirmation OTP",
		Description: "Requires a verified KYC token and an active source account. Idempotency-Key must be a UUID: reuse it with the same body for retries. Minimum amount is 1000 whole VND; fee is currently 0; note is at most 100 characters. Initial status is awaiting_otp; retries can return the existing terminal status. OTP expires after 5 minutes, with 3 attempts and a 30-second resend cooldown. No funds are debited until confirmation.",
		Parameters:  []openapi.Parameter{{Name: "Idempotency-Key", In: "header", Required: true, Type: "string", Format: "uuid"}},
		Request:     createTransferRequest{}, Response: createTransferResponse{}, SuccessStatus: http.StatusCreated,
		FailureStatuses: []int{400, 403, 404, 409, 422, 429, 503},
	})
	auth("POST /api/transfers/{transferId}/confirm", h.ConfirmTransfer, openapi.Operation{
		ID: "confirmTransfer", Summary: "Confirm a transfer with email OTP",
		Description: "Send otp as a string. Success returns the outgoing transaction record immediately from canonical state. Retry the same transferId after a timeout; an already successful transfer returns the same result without another debit. INVALID_OTP returns attemptsLeft; OTP_EXPIRED (410) and OTP_ATTEMPTS_EXCEEDED (423) cancel the pending transfer. Insufficient funds at confirmation returns 400.",
		Request:     confirmRequest{}, Response: transactionResponseDoc{}, FailureStatuses: []int{400, 404, 409, 410, 423},
	})
	auth("POST /api/transfers/{transferId}/otp/resend", h.ResendOTP, openapi.Operation{
		ID: "resendTransferOTP", Summary: "Resend OTP for a pending transfer",
		Description: "No body required. Use resendAvailableAt for the countdown. A successful resend issues new OTP metadata; attemptsLeft resets to 3. Terminal transfers return 409, early resend returns 429.",
		Response:    otp.TransferOTPInfo{}, FailureStatuses: []int{404, 409, 429},
	})
	auth("GET /api/transfers/recent-recipients", h.RecentRecipients, openapi.Operation{
		ID: "recentTransferRecipients", Summary: "List recent successful transfer recipients",
		Description: "limit defaults to 4 and is capped at 10. Deduplicates recipients from up to the 100 latest successful outgoing feed records. Eventually consistent Mongo projection.",
		Parameters:  []openapi.Parameter{limit("4")}, Response: struct {
			Items []struct {
				AccountNo         string    `json:"accountNo"`
				Name              string    `json:"name"`
				BankCode          string    `json:"bankCode"`
				LastTransferredAt time.Time `json:"lastTransferredAt"`
			} `json:"items"`
		}{}, FailureStatuses: []int{404},
	})
	auth("GET /api/transactions", h.ListTransactions, openapi.Operation{
		ID: "listTransactions", Summary: "List and search transaction history",
		Description: "direction: all/in/out (default all). q searches counterparty, note, reference and amount, case/accent insensitive. from/to are RFC3339 inclusive bounds. limit defaults to 20 and is capped at 50. Pass the opaque nextCursor unchanged; keep filters unchanged between pages. Sorted newest first; nextCursor null means end. Read projection is eventually consistent. Status is success/failed; pending transfers are not included in this feed.",
		Parameters:  []openapi.Parameter{query("direction", "all", false), query("q", "chuyen tien", false), {Name: "from", In: "query", Type: "string", Format: "date-time"}, {Name: "to", In: "query", Type: "string", Format: "date-time"}, limit("20"), query("cursor", "", false)},
		Response: struct {
			Items      []transactionResponseDoc `json:"items"`
			NextCursor *string                  `json:"nextCursor"`
		}{}, FailureStatuses: []int{400, 404},
	})
	auth("GET /api/transactions/summary", h.TransactionSummary, openapi.Operation{
		ID: "transactionSummary", Summary: "Get monthly incoming and outgoing totals",
		Description: "month is required in YYYY-MM format. Calendar boundaries use UTC+07:00. Totals include successful transaction amounts; outgoing totals exclude fees. Eventually consistent projection.",
		Parameters:  []openapi.Parameter{query("month", "2026-10", true)}, Response: struct {
			Month    string `json:"month"`
			TotalIn  int64  `json:"totalIn"`
			TotalOut int64  `json:"totalOut"`
		}{}, FailureStatuses: []int{400, 404},
	})
	auth("GET /api/transactions/{id}", h.TransactionDetail, openapi.Operation{
		ID: "transactionDetail", Summary: "Get an owned transaction's details",
		Description: "id is a transaction/transfer UUID. Canonical successful or cancelled state is available before Mongo catches up. Pending or non-owned transactions return 404. balanceAfter is absent for cancelled transactions; cancelled maps to failed.",
		Response:    transactionResponseDoc{}, FailureStatuses: []int{404},
	})
}
