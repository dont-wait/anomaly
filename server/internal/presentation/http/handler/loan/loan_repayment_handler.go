package loan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	accountqueries "github.com/dont-wait/anomaly/internal/application/account/queries"
	commands "github.com/dont-wait/anomaly/internal/application/loan/commands"
	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	"github.com/google/uuid"
)

const repaymentIdempotencyTTL = 24 * time.Hour

type RepaymentIdempotencyCache interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte, time.Duration) error
	SetNX(context.Context, string, []byte, time.Duration) (bool, error)
}

type RepaymentHTTP struct {
	Queries     *loanqueries.RepaymentQueryService
	Commands    *commands.RepayLoanCommandHandler
	Idempotency RepaymentIdempotencyCache
}

type repaymentError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

type repaymentQuoteRequest struct {
	InstallmentNo   int    `json:"installmentNo"`
	SourceAccountID string `json:"sourceAccountId"`
}

type repaymentConfirmRequest struct {
	QuoteID         string `json:"quoteId"`
	SourceAccountID string `json:"sourceAccountId"`
}

type repaymentResponse struct {
	PaymentID                 string    `json:"paymentId"`
	LoanID                    string    `json:"loanId"`
	TransactionID             string    `json:"transactionId"`
	InstallmentNo             int       `json:"installmentNo"`
	Status                    string    `json:"status"`
	Amount                    int64     `json:"amount"`
	PrincipalAmount           int64     `json:"principalAmount"`
	InterestAmount            int64     `json:"interestAmount"`
	PenaltyAmount             int64     `json:"penaltyAmount"`
	OutstandingPrincipalAfter int64     `json:"outstandingPrincipalAfter"`
	LoanStatus                string    `json:"loanStatus"`
	PaidAt                    time.Time `json:"paidAt"`
}

type repaymentIdempotencyEntry struct {
	Fingerprint string             `json:"fingerprint"`
	CustomerID  string             `json:"customerId"`
	LoanID      string             `json:"loanId"`
	Quote       loanqueries.Quote  `json:"quote"`
	Result      *repaymentResponse `json:"result,omitempty"`
}

func repaymentWriteError(
	w http.ResponseWriter,
	status int,
	code, message string,
) {
	httpx.WriteJSON(w, status, repaymentError{
		Error: message,
		Code:  code,
	})
}

func repaymentHandleError(
	w http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(err, loanqueries.ErrLoanNotFound),
		errors.Is(err, loandomain.ErrLoanNotFound),
		errors.Is(err, loanqueries.ErrPaymentNotFound):
		repaymentWriteError(w, 404, "LOAN_NOT_FOUND",
			"Loan or payment not found")

	case errors.Is(err, loanqueries.ErrInvalidPaymentAccount):
		repaymentWriteError(w, 422, "INVALID_PAYMENT_ACCOUNT",
			"Payment account is not eligible")

	case errors.Is(err, loanqueries.ErrInvalidRepaymentQuery):
		repaymentWriteError(w, 400, "INVALID_REQUEST",
			"Invalid repayment request")

	case errors.Is(err, loanqueries.ErrInvalidInstallmentOrder),
		errors.Is(err, loandomain.ErrInvalidInstallmentOrder):
		repaymentWriteError(w, 422, "INVALID_INSTALLMENT_ORDER",
			"Installments must be paid in order")

	case errors.Is(err, loanqueries.ErrInstallmentAlreadyPaid),
		errors.Is(err, loandomain.ErrInstallmentAlreadyPaid),
		errors.Is(err, loandomain.ErrLoanRepaymentCompleted):
		repaymentWriteError(w, 409, "INSTALLMENT_ALREADY_PAID",
			"Installment already paid or loan completed")

	case errors.Is(err, loanqueries.ErrInstallmentNotDue),
		errors.Is(err, loandomain.ErrInstallmentNotDue):
		repaymentWriteError(w, 422, "INSTALLMENT_NOT_DUE",
			"Payment is allowed on the due date only")

	case errors.Is(err, loanqueries.ErrInsufficientBalance),
		errors.Is(err, accountdomain.ErrInsufficientFunds):
		repaymentWriteError(w, 400, "INSUFFICIENT_FUNDS",
			"Insufficient account balance")

	case errors.Is(err, loanqueries.ErrQuoteNotFound):
		repaymentWriteError(w, 410, "QUOTE_EXPIRED",
			"Payment quote has expired")

	case errors.Is(err, loandomain.ErrLoanNotDisbursed):
		repaymentWriteError(w, 422, "LOAN_NOT_ACTIVE",
			"Loan has not been disbursed")

	default:
		repaymentWriteError(w, 503, "PAYMENT_UNAVAILABLE",
			"Loan repayment service unavailable")
	}
}

func repaymentDecodeJSON(
	r *http.Request,
	dest any,
) error {
	if r.Body == nil {
		return fmt.Errorf("request body is required")
	}

	decoder := json.NewDecoder(
		io.LimitReader(r.Body, 1<<20),
	)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dest); err != nil {
		return err
	}

	var extra any

	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("request must contain one JSON object")
	}

	return nil
}

func repaymentActor(
	w http.ResponseWriter,
	r *http.Request,
) (string, bool) {
	claims, ok := middleware.ClaimsFromContext(
		r.Context(),
	)

	if !ok || claims == nil || claims.UserID == "" {
		repaymentWriteError(w, 401, "UNAUTHORIZED",
			"Authentication required")
		return "", false
	}

	return claims.UserID, true
}

func repaymentLoanID(
	w http.ResponseWriter,
	r *http.Request,
) (string, bool) {
	id := r.PathValue("id")

	if _, err := uuid.Parse(id); err != nil {
		repaymentWriteError(w, 400, "INVALID_LOAN_ID",
			"Invalid loan ID")
		return "", false
	}

	return id, true
}

// ID API cua loan_payments phai trung voi
// ObjectId duoc tao trong MongoDB Projection.
func repaymentReadPaymentID(eventPaymentID string) string {
	sum := sha256.Sum256(
		[]byte("anomalybank:loan-payment:" + eventPaymentID),
	)
	return hex.EncodeToString(sum[:12])
}

func repaymentResult(
	result *commands.RepayLoanResult,
) repaymentResponse {
	p := result.Payment

	return repaymentResponse{
		PaymentID:                 repaymentReadPaymentID(p.PaymentID),
		LoanID:                    p.LoanID,
		TransactionID:             p.TransactionID,
		InstallmentNo:             p.InstallmentNo,
		Status:                    "success",
		Amount:                    p.Amount,
		PrincipalAmount:           p.PrincipalAmount,
		InterestAmount:            p.InterestAmount,
		PenaltyAmount:             0,
		OutstandingPrincipalAfter: p.OutstandingPrincipalAfter,
		LoanStatus:                string(result.Status),
		PaidAt:                    p.PaidAt,
	}
}

func repaymentFingerprint(
	loanID string,
	req repaymentConfirmRequest,
) string {
	sum := sha256.Sum256(
		[]byte(loanID + "\x00" +
			req.QuoteID + "\x00" +
			req.SourceAccountID),
	)
	return hex.EncodeToString(sum[:])
}

func (h *RepaymentHTTP) RegisterRoutes(
	router *openapi.Registry,
	tokens accountqueries.TokenService,
) {
	auth := func(
		pattern string,
		fn http.HandlerFunc,
		id string,
		request any,
		response any,
	) {
		protected := middleware.RequireAuth(tokens)(
			http.HandlerFunc(func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.Header().Set("Cache-Control", "no-store")
				fn(w, r)
			}),
		)

		op := openapi.Operation{
			ID:              id,
			Tags:            []string{"Loan Repayment"},
			Auth:            true,
			Request:         request,
			Response:        response,
			FailureStatuses: []int{400, 401, 404, 409, 410, 422, 503},
		}

		if id == "listLoanPayments" {
			op.Description = "Descending by installment number. cursor is the last installmentNo returned (exclusive), not a numeric offset; keep limit unchanged between pages."
			op.Parameters = []openapi.Parameter{
				{Name: "limit", In: "query", Type: "integer", Example: "20"},
				{Name: "cursor", In: "query", Type: "string", Example: "4"},
			}
		}

		if id == "payLoanInstallment" {
			op.Parameters = []openapi.Parameter{
				{
					Name:     "Idempotency-Key",
					In:       "header",
					Type:     "string",
					Format:   "uuid",
					Required: true,
				},
			}
		}

		router.Handle(pattern, protected, op)
	}

	auth(
		"GET /api/loans/{id}/repayment-schedule",
		h.schedule,
		"getLoanRepaymentSchedule",
		nil,
		loanqueries.Schedule{},
	)
	auth(
		"POST /api/loans/{id}/repayments/quote",
		h.quote,
		"quoteLoanRepayment",
		repaymentQuoteRequest{},
		loanqueries.Quote{},
	)
	auth(
		"POST /api/loans/{id}/repayments",
		h.repay,
		"payLoanInstallment",
		repaymentConfirmRequest{},
		repaymentResponse{},
	)
	// Cursor is the last installmentNo returned, not a numeric offset.
	// Expose query params through the common authenticated route registration.
	auth(
		"GET /api/loans/{id}/payments",
		h.payments,
		"listLoanPayments",
		nil,
		struct {
			Items      []loanqueries.Payment `json:"items"`
			NextCursor *string               `json:"nextCursor"`
		}{},
	)
	auth(
		"GET /api/loans/{id}/payments/{paymentId}",
		h.payment,
		"getLoanPayment",
		nil,
		loanqueries.Payment{},
	)
}

// 1. GET repayment-schedule.
func (h *RepaymentHTTP) schedule(
	w http.ResponseWriter,
	r *http.Request,
) {
	customerID, ok := repaymentActor(w, r)
	if !ok {
		return
	}

	loanID, ok := repaymentLoanID(w, r)
	if !ok {
		return
	}

	if h == nil || h.Queries == nil {
		repaymentWriteError(w, 503, "PAYMENT_UNAVAILABLE",
			"Repayment query unavailable")
		return
	}

	result, err := h.Queries.Schedule(
		r.Context(), loanID, customerID,
	)
	if err != nil {
		repaymentHandleError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, result)
}

// 2. POST repayments/quote.
func (h *RepaymentHTTP) quote(
	w http.ResponseWriter,
	r *http.Request,
) {
	customerID, ok := repaymentActor(w, r)
	if !ok {
		return
	}

	loanID, ok := repaymentLoanID(w, r)
	if !ok {
		return
	}

	var req repaymentQuoteRequest

	if err := repaymentDecodeJSON(r, &req); err != nil {
		repaymentWriteError(w, 400, "INVALID_REQUEST",
			"Invalid JSON request")
		return
	}

	if h == nil || h.Queries == nil {
		repaymentWriteError(w, 503, "PAYMENT_UNAVAILABLE",
			"Repayment query unavailable")
		return
	}

	result, err := h.Queries.Quote(
		r.Context(),
		loanID,
		customerID,
		req.SourceAccountID,
		req.InstallmentNo,
	)
	if err != nil {
		repaymentHandleError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, result)
}

// 3. POST repayments.
func (h *RepaymentHTTP) repay(
	w http.ResponseWriter,
	r *http.Request,
) {
	customerID, ok := repaymentActor(w, r)
	if !ok {
		return
	}

	loanID, ok := repaymentLoanID(w, r)
	if !ok {
		return
	}

	key := r.Header.Get("Idempotency-Key")

	if _, err := uuid.Parse(key); err != nil {
		repaymentWriteError(w, 400, "INVALID_IDEMPOTENCY_KEY",
			"Idempotency-Key must be a UUID")
		return
	}

	var req repaymentConfirmRequest

	if err := repaymentDecodeJSON(r, &req); err != nil {
		repaymentWriteError(w, 400, "INVALID_REQUEST",
			"Invalid JSON request")
		return
	}

	if req.QuoteID == "" ||
		req.SourceAccountID == "" {
		repaymentWriteError(w, 422, "INVALID_PAYMENT_ACCOUNT",
			"Invalid repayment account or quote")
		return
	}

	if h == nil || h.Queries == nil ||
		h.Commands == nil || h.Idempotency == nil {
		repaymentWriteError(w, 503, "PAYMENT_UNAVAILABLE",
			"Repayment service unavailable")
		return
	}

	ctx := r.Context()
	cacheKey := "loan-repayment:idempotency:" +
		customerID + ":" + key

	fingerprint := repaymentFingerprint(loanID, req)

	data, found, err := h.Idempotency.Get(ctx, cacheKey)
	if err != nil {
		repaymentHandleError(w, err)
		return
	}

	var entry repaymentIdempotencyEntry

	if found {
		if json.Unmarshal(data, &entry) != nil {
			repaymentWriteError(w, 503, "PAYMENT_UNAVAILABLE",
				"Invalid idempotency state")
			return
		}

		if entry.Fingerprint != fingerprint ||
			entry.LoanID != loanID ||
			entry.CustomerID != customerID {
			repaymentWriteError(w, 409, "IDEMPOTENCY_CONFLICT",
				"Idempotency-Key was used with another request")
			return
		}

		if entry.Result != nil {
			httpx.WriteJSON(w, 200, entry.Result)
			return
		}
	} else {
		stored, err := h.Queries.GetQuote(
			ctx, req.QuoteID, customerID,
		)
		if err != nil {
			repaymentHandleError(w, err)
			return
		}

		if stored.Quote.LoanID != loanID ||
			stored.SourceAccountID != req.SourceAccountID {
			repaymentWriteError(w, 422, "INVALID_PAYMENT_ACCOUNT",
				"Quote does not match loan and account")
			return
		}

		entry = repaymentIdempotencyEntry{
			Fingerprint: fingerprint,
			CustomerID:  customerID,
			LoanID:      loanID,
			Quote:       stored.Quote,
		}

		raw, err := json.Marshal(entry)
		if err != nil {
			repaymentHandleError(w, err)
			return
		}

		created, err := h.Idempotency.SetNX(
			ctx, cacheKey, raw, repaymentIdempotencyTTL,
		)
		if err != nil {
			repaymentHandleError(w, err)
			return
		}

		// Hai request dong thoi su dung cung key.
		if !created {
			other, exists, err := h.Idempotency.Get(
				ctx, cacheKey,
			)
			if err != nil || !exists ||
				json.Unmarshal(other, &entry) != nil {
				repaymentWriteError(w, 503, "PAYMENT_UNAVAILABLE",
					"Idempotency state unavailable")
				return
			}

			if entry.Fingerprint != fingerprint ||
				entry.CustomerID != customerID ||
				entry.LoanID != loanID {
				repaymentWriteError(w, 409, "IDEMPOTENCY_CONFLICT",
					"Idempotency-Key conflict")
				return
			}

			if entry.Result != nil {
				httpx.WriteJSON(w, 200, entry.Result)
				return
			}
		}
	}

	// Neu Quote het han, chi chap nhan retry
	// khi Banking Stream DA ghi nhan ky nay.
	if !time.Now().UTC().Before(
		entry.Quote.QuoteExpiresAt,
	) {
		history, err := h.Commands.Bank.ListLoanPayments(
			ctx, loanID,
		)
		if err != nil {
			repaymentHandleError(w, err)
			return
		}

		alreadyPaid := false

		for _, item := range history {
			if item.InstallmentNo == entry.Quote.InstallmentNo &&
				item.CustomerID == customerID &&
				item.AccountID == req.SourceAccountID {
				alreadyPaid = true
				break
			}
		}

		if !alreadyPaid {
			repaymentWriteError(w, 410, "QUOTE_EXPIRED",
				"Quote expired; create a new quote")
			return
		}
	}

	// Command chi nhan ID va so ky.
	// So tien luon duoc lay lai tu lich EMI,
	// khong nhan principal/interest tu client.
	result, err := h.Commands.Handle(
		ctx,
		commands.RepayLoanCommand{
			LoanID:          loanID,
			CustomerID:      customerID,
			SourceAccountID: req.SourceAccountID,
			InstallmentNo:   entry.Quote.InstallmentNo,
		},
	)
	if err != nil {
		repaymentHandleError(w, err)
		return
	}

	response := repaymentResult(result)
	entry.Result = &response

	raw, err := json.Marshal(entry)
	if err == nil {
		// Neu luu response vao Redis loi, Banking
		// van da commit. Lan retry se duoc
		// replay tu Banking Stream, khong tru lai.
		_ = h.Idempotency.Set(
			ctx, cacheKey, raw, repaymentIdempotencyTTL,
		)
	}

	httpx.WriteJSON(w, 200, response)
}

// 4. GET payments.
func (h *RepaymentHTTP) payments(
	w http.ResponseWriter,
	r *http.Request,
) {
	customerID, ok := repaymentActor(w, r)
	if !ok {
		return
	}

	loanID, ok := repaymentLoanID(w, r)
	if !ok {
		return
	}

	if h == nil || h.Queries == nil {
		repaymentWriteError(w, 503, "PAYMENT_UNAVAILABLE",
			"Repayment query unavailable")
		return
	}

	limit := 20
	afterInstallment := 0

	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			repaymentWriteError(w, 400, "INVALID_LIMIT",
				"Limit must be between 1 and 50")
			return
		}
		limit = n
	}

	if raw := r.URL.Query().Get("cursor"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 60 {
			repaymentWriteError(w, 400, "INVALID_CURSOR",
				"Invalid installment cursor")
			return
		}
		afterInstallment = n
	}

	items, err := h.Queries.Payments(
		r.Context(), loanID, customerID,
	)
	if err != nil {
		repaymentHandleError(w, err)
		return
	}

	page, next := repaymentHistoryPage(items, afterInstallment, limit)
	httpx.WriteJSON(w, 200, map[string]any{
		"items": page, "nextCursor": next,
	})
}

// 5. GET payments/{paymentId}.
func (h *RepaymentHTTP) payment(
	w http.ResponseWriter,
	r *http.Request,
) {
	customerID, ok := repaymentActor(w, r)
	if !ok {
		return
	}

	loanID, ok := repaymentLoanID(w, r)
	if !ok {
		return
	}

	if h == nil || h.Queries == nil {
		repaymentWriteError(w, 503, "PAYMENT_UNAVAILABLE",
			"Repayment query unavailable")
		return
	}

	paymentID := strings.TrimSpace(
		r.PathValue("paymentId"),
	)

	if len(paymentID) != 24 {
		repaymentWriteError(w, 404, "PAYMENT_NOT_FOUND",
			"Payment not found")
		return
	}

	result, err := h.Queries.Payment(
		r.Context(),
		loanID,
		customerID,
		paymentID,
	)
	if err != nil {
		if errors.Is(err, loanqueries.ErrPaymentNotFound) {
			repaymentWriteError(w, 404, "PAYMENT_NOT_FOUND",
				"Payment not found")
			return
		}
		repaymentHandleError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, result)
}

// Keyset pagination: a new installment at the head never shifts older pages.
// Reader guarantees items ordered by installmentNo DESC.
func repaymentHistoryPage(items []loanqueries.Payment, afterInstallment, limit int) ([]loanqueries.Payment, *string) {
	start := 0
	if afterInstallment != 0 {
		for start < len(items) && items[start].InstallmentNo >= afterInstallment {
			start++
		}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	var next *string
	if end < len(items) && len(page) > 0 {
		value := strconv.Itoa(page[len(page)-1].InstallmentNo)
		next = &value
	}
	return page, next
}
