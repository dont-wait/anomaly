package transaction

import (
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/text/unicode/norm"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/infrastructure/otp"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type Handler struct {
	logger zerolog.Logger
	repo   *mongorepo.TransferRepository
	otp    *otp.TransferOTPStore
}

func NewHandler(logger zerolog.Logger, repo *mongorepo.TransferRepository, otpStore *otp.TransferOTPStore) *Handler {
	return &Handler{logger: logger, repo: repo, otp: otpStore}
}

// Register registers endpoints behind the same JWT middleware used by account APIs.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokenSvc queries.TokenService) {
	auth := func(pattern string, fn http.HandlerFunc) {
		secured := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			fn.ServeHTTP(w, r)
		})
		mux.Handle(pattern, middleware.RequireAuth(tokenSvc)(secured))
	}
	auth("GET /api/accounts/lookup", h.Lookup)
	auth("GET /api/transfers/recent-recipients", h.RecentRecipients)
	auth("POST /api/transfers", h.CreateTransfer)
	auth("POST /api/transfers/{transferId}/confirm", h.ConfirmTransfer)
	auth("POST /api/transfers/{transferId}/otp/resend", h.ResendOTP)
	auth("GET /api/transactions", h.ListTransactions)
	auth("GET /api/transactions/summary", h.TransactionSummary)
	auth("GET /api/transactions/{id}", h.TransactionDetail)
}

var accountNumberPattern = regexp.MustCompile(`^\d{6,19}$`)

func (h *Handler) Lookup(w http.ResponseWriter, r *http.Request) {
	bankCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("bankCode")))
	accountNo := strings.TrimSpace(r.URL.Query().Get("accountNo"))
	if bankCode != "ANOMALY" {
		writeAPIError(w, http.StatusBadRequest, "UNSUPPORTED_BANK", "unsupported bank")
		return
	}
	if !accountNumberPattern.MatchString(accountNo) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_ACCOUNT_NO", "invalid account number")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	source, err := h.repo.FindOwnedAccount(r.Context(), claims.UserID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	recipient, err := h.repo.FindRecipient(r.Context(), accountNo)
	if errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		writeAPIError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	if source.ID == recipient.ID {
		writeAPIError(w, http.StatusUnprocessableEntity, "SELF_TRANSFER", "self transfer is not allowed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"accountNo": recipient.AccountNo, "name": recipient.Name, "bankCode": "ANOMALY"})
}

type createTransferRequest struct {
	ToBankCode  string `json:"toBankCode"`
	ToAccountNo string `json:"toAccountNo"`
	Amount      int64  `json:"amount"`
	Note        string `json:"note"`
}

type createTransferResponse struct {
	TransferID string `json:"transferId"`
	Status     string `json:"status"`
	Recipient  struct {
		AccountNo string `json:"accountNo"`
		Name      string `json:"name"`
		BankCode  string `json:"bankCode"`
	} `json:"recipient"`
	Amount int64               `json:"amount"`
	Fee    int64               `json:"fee"`
	Note   string              `json:"note"`
	OTP    otp.TransferOTPInfo `json:"otp"`
}

func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	if !claims.IsVerify {
		writeAPIError(w, http.StatusForbidden, "KYC_REQUIRED", "account verification required")
		return
	}
	var req createTransferRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.ToBankCode = strings.ToUpper(strings.TrimSpace(req.ToBankCode))
	req.ToAccountNo = strings.TrimSpace(req.ToAccountNo)
	req.Note = strings.TrimSpace(req.Note)
	if req.ToBankCode != "ANOMALY" {
		writeAPIError(w, http.StatusBadRequest, "UNSUPPORTED_BANK", "unsupported bank")
		return
	}
	if !accountNumberPattern.MatchString(req.ToAccountNo) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_ACCOUNT_NO", "invalid account number")
		return
	}
	if req.Amount < 1000 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_AMOUNT", "minimum transfer amount is 1000 VND")
		return
	}
	if len([]rune(req.Note)) > 100 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_NOTE", "note must be at most 100 characters")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if _, err := uuid.Parse(key); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must be a UUID")
		return
	}
	source, err := h.repo.FindOwnedAccount(r.Context(), claims.UserID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	if source.Status != "active" {
		writeAPIError(w, http.StatusForbidden, "ACCOUNT_INACTIVE", "account is not active")
		return
	}
	recipient, err := h.repo.FindRecipient(r.Context(), req.ToAccountNo)
	if errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		writeAPIError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	if source.ID == recipient.ID {
		writeAPIError(w, http.StatusUnprocessableEntity, "SELF_TRANSFER", "self transfer is not allowed")
		return
	}
	if source.Balance < req.Amount {
		writeAPIError(w, http.StatusBadRequest, "INSUFFICIENT_FUNDS", "insufficient funds")
		return
	}
	id := uuid.NewString()
	compact := strings.ReplaceAll(id, "-", "")
	tx := mongorepo.TransferDocument{ID: id, Reference: "FT" + strings.ToUpper(compact[len(compact)-11:]), Type: "transfer", Amount: req.Amount, Fee: 0, Currency: "VND", Channel: "web", Status: "awaiting_otp", Note: req.Note, IdempotencyKey: key, CreatedAt: time.Now().UTC(), OTPExpiresAt: time.Now().UTC().Add(5 * time.Minute), OTPResendAt: time.Now().UTC().Add(30 * time.Second)}
	tx.Source.AccountID, tx.Source.AccountNo, tx.Source.Name = source.ID, source.AccountNo, source.Name
	tx.Destination.Type, tx.Destination.AccountID, tx.Destination.AccountNo = "internal", recipient.ID, recipient.AccountNo
	tx.Destination.BankCode, tx.Destination.Name = "ANOMALY", recipient.Name
	tx, created, err := h.repo.CreatePending(r.Context(), tx)
	if errors.Is(err, mongorepo.ErrTransactionConflict) {
		writeAPIError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with an existing transfer")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	response := createTransferResponse{TransferID: tx.ID, Status: tx.Status, Amount: tx.Amount, Fee: tx.Fee, Note: tx.Note}
	response.Recipient.AccountNo, response.Recipient.Name, response.Recipient.BankCode = tx.Destination.AccountNo, tx.Destination.Name, tx.Destination.BankCode
	if created {
		info, err := h.otp.Issue(r.Context(), tx.ID, source.Email)
		if err != nil {
			_ = h.repo.CancelPending(r.Context(), tx.ID)
			if errors.Is(err, otp.ErrTransferOTPTooSoon) {
				writeAPIError(w, http.StatusTooManyRequests, "OTP_RESEND_TOO_SOON", "otp request is not available yet")
				return
			}
			writeAPIError(w, http.StatusServiceUnavailable, "OTP_DELIVERY_FAILED", "could not send confirmation code")
			return
		}
		if err := h.repo.SetOTPMetadata(r.Context(), tx.ID, info.ExpiresAt, info.ResendAvailableAt); err != nil {
			h.writeRepoError(w, err)
			return
		}
		response.OTP = info
	} else {
		response.OTP = otp.TransferOTPInfo{Channel: "email", MaskedDestination: maskEmail(source.Email), ExpiresAt: tx.OTPExpiresAt, AttemptsLeft: 3, ResendAvailableAt: tx.OTPResendAt}
	}
	httpx.WriteJSON(w, http.StatusCreated, response)
}

type confirmRequest struct {
	OTP string `json:"otp"`
}

func (h *Handler) ConfirmTransfer(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	id := r.PathValue("transferId")
	tx, err := h.repo.FindTransfer(r.Context(), id, claims.UserID)
	if errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		writeAPIError(w, http.StatusNotFound, "TRANSFER_NOT_FOUND", "transfer not found")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	accountID, err := h.repo.AccountObjectID(r.Context(), claims.UserID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	if tx.Status == "success" {
		feed, err := h.repo.GetFeed(r.Context(), accountID, tx.ID)
		if err != nil {
			h.writeRepoError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, transactionResponse(feed))
		return
	}
	var req confirmRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	attemptsLeft, err := h.otp.Verify(r.Context(), id, req.OTP)
	if errors.Is(err, otp.ErrTransferOTPInvalid) {
		writeAPIErrorExtra(w, http.StatusBadRequest, "INVALID_OTP", "invalid otp", map[string]int{"attemptsLeft": attemptsLeft})
		return
	}
	if errors.Is(err, otp.ErrTransferOTPExpired) {
		_ = h.repo.CancelPending(r.Context(), id)
		writeAPIError(w, http.StatusGone, "OTP_EXPIRED", "otp expired")
		return
	}
	if errors.Is(err, otp.ErrTransferOTPExceeded) {
		_ = h.repo.CancelPending(r.Context(), id)
		writeAPIError(w, http.StatusLocked, "OTP_ATTEMPTS_EXCEEDED", "otp attempts exceeded")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	confirmed, err := h.repo.ConfirmTransfer(r.Context(), id, claims.UserID)
	if errors.Is(err, mongorepo.ErrTransactionInsufficient) {
		writeAPIError(w, http.StatusBadRequest, "INSUFFICIENT_FUNDS", "insufficient funds")
		return
	}
	if errors.Is(err, mongorepo.ErrTransactionBalanceOverflow) {
		writeAPIError(w, http.StatusConflict, "BALANCE_LIMIT_EXCEEDED", "destination balance limit exceeded")
		return
	}
	if errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		writeAPIError(w, http.StatusNotFound, "TRANSFER_NOT_FOUND", "transfer not found")
		return
	}
	if errors.Is(err, mongorepo.ErrTransactionConflict) {
		writeAPIError(w, http.StatusConflict, "TRANSFER_ALREADY_PROCESSED", "transfer already processed")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	_ = h.otp.Consume(r.Context(), id)
	feed, err := h.repo.GetFeed(r.Context(), accountID, confirmed.ID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, transactionResponse(feed))
}

func (h *Handler) ResendOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	id := r.PathValue("transferId")
	tx, err := h.repo.FindTransfer(r.Context(), id, claims.UserID)
	if errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		writeAPIError(w, http.StatusNotFound, "TRANSFER_NOT_FOUND", "transfer not found")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	if tx.Status != "awaiting_otp" {
		writeAPIError(w, http.StatusConflict, "TRANSFER_ALREADY_PROCESSED", "transfer already processed")
		return
	}
	account, err := h.repo.FindOwnedAccount(r.Context(), claims.UserID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	info, err := h.otp.Resend(r.Context(), id, account.Email)
	if errors.Is(err, otp.ErrTransferOTPTooSoon) {
		writeAPIError(w, http.StatusTooManyRequests, "OTP_RESEND_TOO_SOON", "otp resend is not available yet")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	if err := h.repo.SetOTPMetadata(r.Context(), id, info.ExpiresAt, info.ResendAvailableAt); err != nil {
		h.writeRepoError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, info)
}

func (h *Handler) RecentRecipients(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	accountID, err := h.repo.AccountObjectID(r.Context(), claims.UserID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 4, 10)
	rows, err := h.repo.ListFeed(r.Context(), accountID, bson.M{"direction": "OUT", "type": "transfer", "status": "success"}, 100)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	type recentRecipient struct {
		AccountNo         string    `json:"accountNo"`
		Name              string    `json:"name"`
		BankCode          string    `json:"bankCode"`
		LastTransferredAt time.Time `json:"lastTransferredAt"`
	}
	items := make([]recentRecipient, 0, limit)
	seen := map[string]bool{}
	for _, row := range rows {
		key := row.Counterparty.BankCode + ":" + row.Counterparty.AccountNo
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, recentRecipient{row.Counterparty.AccountNo, row.Counterparty.Name, row.Counterparty.BankCode, row.OccurredAt})
		if len(items) == limit {
			break
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	accountID, err := h.repo.AccountObjectID(r.Context(), claims.UserID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 20, 50)
	filter := bson.M{}
	switch strings.ToLower(r.URL.Query().Get("direction")) {
	case "", "all":
	case "in":
		filter["direction"] = "IN"
	case "out":
		filter["direction"] = "OUT"
	default:
		writeAPIError(w, http.StatusBadRequest, "INVALID_DIRECTION", "invalid direction")
		return
	}
	if q := normalizeSearch(r.URL.Query().Get("q")); q != "" {
		filter["search_text"] = bson.M{"$regex": regexp.QuoteMeta(q)}
	}
	dateFilter := bson.M{}
	for key, op := range map[string]string{"from": "$gte", "to": "$lte"} {
		if raw := r.URL.Query().Get(key); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "INVALID_DATE", "from and to must use RFC3339")
				return
			}
			dateFilter[op] = parsed
		}
	}
	if len(dateFilter) > 0 {
		filter["occurred_at"] = dateFilter
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, err := mongorepo.DecodeFeedCursor(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_CURSOR", "invalid cursor")
			return
		}
		cursorID, _ := bson.ObjectIDFromHex(cursor.ID)
		filter["$or"] = bson.A{bson.M{"occurred_at": bson.M{"$lt": cursor.OccurredAt}}, bson.M{"occurred_at": cursor.OccurredAt, "_id": bson.M{"$lt": cursorID}}}
	}
	rows, err := h.repo.ListFeed(r.Context(), accountID, filter, int64(limit+1))
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	next := ""
	if len(rows) > limit {
		next = mongorepo.FeedCursorFor(rows[limit-1])
		rows = rows[:limit]
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, transactionResponse(row))
	}
	var nextCursor any
	if next != "" {
		nextCursor = next
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": nextCursor})
}

func (h *Handler) TransactionSummary(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	parsed, err := time.Parse("2006-01", month)
	if err != nil || parsed.Format("2006-01") != month {
		writeAPIError(w, http.StatusBadRequest, "INVALID_MONTH", "month must use YYYY-MM")
		return
	}
	location := time.FixedZone("ICT", 7*60*60)
	from := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, location)
	to := from.AddDate(0, 1, 0)
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	accountID, err := h.repo.AccountObjectID(r.Context(), claims.UserID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	in, out, err := h.repo.Summary(r.Context(), accountID, from.UTC(), to.UTC())
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"month": month, "totalIn": in, "totalOut": out})
}

func (h *Handler) TransactionDetail(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	accountID, err := h.repo.AccountObjectID(r.Context(), claims.UserID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	row, err := h.repo.GetFeed(r.Context(), accountID, r.PathValue("id"))
	if errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		writeAPIError(w, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "transaction not found")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, transactionResponse(row))
}

func transactionResponse(row mongorepo.FeedDocument) map[string]any {
	direction := strings.ToLower(row.Direction)
	status := row.Status
	if status == "awaiting_otp" {
		status = "pending"
	}
	if status == "cancelled" {
		status = "failed"
	}
	counterparty := map[string]any{"name": row.Counterparty.Name, "accountNo": row.Counterparty.AccountNo, "bankCode": row.Counterparty.BankCode}
	if row.Counterparty.BankCode == "ANOMALY" {
		counterparty["bank"] = "AnomalyBank"
	}
	result := map[string]any{"id": row.TransactionID, "reference": row.Reference, "direction": direction, "status": status, "kind": row.Type, "amount": row.Amount, "fee": row.Fee, "note": row.Note, "counterparty": counterparty, "createdAt": row.OccurredAt}
	if row.BalanceAfter != nil {
		result["balanceAfter"] = *row.BalanceAfter
	}
	return result
}

func normalizeSearch(value string) string {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "đ", "d"))
	var b strings.Builder
	for _, r := range norm.NFD.String(value) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func parseLimit(raw string, fallback, max int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}

func maskEmail(value string) string {
	address, err := mail.ParseAddress(value)
	if err != nil {
		return "***"
	}
	parts := strings.SplitN(address.Address, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	if len(parts[0]) < 2 {
		return parts[0] + "***@" + parts[1]
	}
	return parts[0][:2] + "***@" + parts[1]
}

func (h *Handler) writeRepoError(w http.ResponseWriter, err error) {
	if errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		writeAPIError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}
	h.logger.Error().Err(err).Msg("transaction request failed")
	writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeAPIErrorExtra(w, status, code, message, nil)
}

func writeAPIErrorExtra(w http.ResponseWriter, status int, code, message string, extra any) {
	body := map[string]any{"error": message, "code": code}
	if fields, ok := extra.(map[string]int); ok {
		for key, value := range fields {
			body[key] = value
		}
	}
	httpx.WriteJSON(w, status, body)
}
