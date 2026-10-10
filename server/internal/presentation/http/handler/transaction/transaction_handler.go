package transaction

import (
	"context"
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

	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/infrastructure/otp"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type Repository interface {
	FindOwnedAccount(context.Context, string) (mongorepo.TransferAccount, error)
	FindRecipient(context.Context, string) (mongorepo.TransferAccount, error)
	AccountObjectID(context.Context, string) (bson.ObjectID, error)
	FindTransferByIdempotencyKey(context.Context, string, string) (mongorepo.TransferDocument, error)
	CreatePending(context.Context, mongorepo.TransferDocument) (mongorepo.TransferDocument, bool, error)
	FindTransfer(context.Context, string, string) (mongorepo.TransferDocument, error)
	ConfirmTransfer(context.Context, string, string) (mongorepo.TransferDocument, error)
	CancelPending(context.Context, string) error
	SetOTPMetadata(context.Context, string, time.Time, time.Time) error
	GetFeed(context.Context, bson.ObjectID, string) (mongorepo.FeedDocument, error)
	ListFeed(context.Context, bson.ObjectID, bson.M, int64) ([]mongorepo.FeedDocument, error)
	Summary(context.Context, bson.ObjectID, time.Time, time.Time) (int64, int64, error)
}

type Handler struct {
	logger zerolog.Logger
	repo   Repository
	otp    *otp.TransferOTPStore
}

func NewHandler(logger zerolog.Logger, repo Repository, otpStore *otp.TransferOTPStore) *Handler {
	return &Handler{logger: logger, repo: repo, otp: otpStore}
}

var accountNumberPattern = regexp.MustCompile(`^\d{6,19}$`)

var transactionLocation = time.FixedZone("ICT", 7*60*60)

func transactionDateBounds(now time.Time) (time.Time, time.Time) {
	now = now.In(transactionLocation)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, transactionLocation)
	return today.AddDate(0, -2, 0), today.AddDate(0, 0, 1).Add(-time.Nanosecond)
}

func parseTransactionDateRange(r *http.Request) (time.Time, time.Time, bool, string, string) {
	rawFrom := r.URL.Query().Get("from")
	rawTo := r.URL.Query().Get("to")
	if rawFrom == "" && rawTo == "" {
		return time.Time{}, time.Time{}, false, "", ""
	}
	if rawFrom == "" || rawTo == "" {
		return time.Time{}, time.Time{}, false, "INVALID_DATE_RANGE", "from and to are required together"
	}
	from, err := time.Parse(time.RFC3339, rawFrom)
	if err != nil {
		return time.Time{}, time.Time{}, false, "INVALID_DATE", "from and to must use RFC3339"
	}
	to, err := time.Parse(time.RFC3339, rawTo)
	if err != nil {
		return time.Time{}, time.Time{}, false, "INVALID_DATE", "from and to must use RFC3339"
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, false, "INVALID_DATE_RANGE", "from must be before or equal to to"
	}
	minDate, maxDate := transactionDateBounds(time.Now())
	if from.Before(minDate) {
		return time.Time{}, time.Time{}, false, "DATE_RANGE_TOO_OLD", "date range cannot start before the last three months"
	}
	if to.After(maxDate) {
		return time.Time{}, time.Time{}, false, "DATE_RANGE_IN_FUTURE", "date range cannot end after today"
	}
	return from, to, true, "", ""
}

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
	Note        string `json:"note,omitempty"`
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

func transferReference(id string) string {
	return "FT" + strings.ToUpper(strings.ReplaceAll(id, "-", ""))
}

func sameCreateRequest(tx mongorepo.TransferDocument, req createTransferRequest) bool {
	return tx.Destination.BankCode == req.ToBankCode && tx.Destination.AccountNo == req.ToAccountNo && tx.Amount == req.Amount && tx.Note == req.Note
}

func createResponse(tx mongorepo.TransferDocument, email string) createTransferResponse {
	response := createTransferResponse{TransferID: tx.ID, Status: tx.Status, Amount: tx.Amount, Fee: tx.Fee, Note: tx.Note}
	response.Recipient.AccountNo, response.Recipient.Name, response.Recipient.BankCode = tx.Destination.AccountNo, tx.Destination.Name, tx.Destination.BankCode
	response.OTP = otp.TransferOTPInfo{Channel: "email", MaskedDestination: maskEmail(email), ExpiresAt: tx.OTPExpiresAt, AttemptsLeft: 3, ResendAvailableAt: tx.OTPResendAt}
	return response
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
	key = strings.ToLower(key)
	existing, err := h.repo.FindTransferByIdempotencyKey(r.Context(), key, claims.UserID)
	if err == nil {
		if !sameCreateRequest(existing, req) {
			writeAPIError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with an existing transfer")
			return
		}
		source, err := h.repo.FindOwnedAccount(r.Context(), claims.UserID)
		if err != nil {
			h.writeRepoError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, createResponse(existing, source.Email))
		return
	}
	if !errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		h.writeRepoError(w, err)
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
	id := uuid.NewString()
	tx := mongorepo.TransferDocument{ID: id, Reference: transferReference(id), Type: "transfer", Amount: req.Amount, Fee: 0, Currency: "VND", Channel: "web", Status: "awaiting_otp", Note: req.Note, IdempotencyKey: key, CreatedAt: time.Now().UTC(), OTPExpiresAt: time.Now().UTC().Add(5 * time.Minute), OTPResendAt: time.Now().UTC().Add(30 * time.Second)}
	tx.Source.AccountID, tx.Source.AccountNo, tx.Source.Name = source.ID, source.AccountNo, source.Name
	tx.Destination.Type, tx.Destination.AccountID, tx.Destination.AccountNo = "internal", recipient.ID, recipient.AccountNo
	tx.Destination.BankCode, tx.Destination.Name = "ANOMALY", recipient.Name
	tx, created, err := h.repo.CreatePending(r.Context(), tx)
	if errors.Is(err, mongorepo.ErrTransactionInsufficient) {
		writeAPIError(w, http.StatusBadRequest, "INSUFFICIENT_FUNDS", "insufficient funds")
		return
	}
	if errors.Is(err, mongorepo.ErrTransactionConflict) {
		writeAPIError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with an existing transfer")
		return
	}
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	response := createResponse(tx, source.Email)
	if created {
		info, err := h.otp.Issue(r.Context(), tx.ID, source.Email)
		if err != nil {
			if cancelErr := h.repo.CancelPending(r.Context(), tx.ID); cancelErr != nil {
				h.writeRepoError(w, cancelErr)
				return
			}
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
		if cancelErr := h.repo.CancelPending(r.Context(), id); cancelErr != nil {
			h.writeRepoError(w, cancelErr)
			return
		}
		writeAPIError(w, http.StatusGone, "OTP_EXPIRED", "otp expired")
		return
	}
	if errors.Is(err, otp.ErrTransferOTPExceeded) {
		if cancelErr := h.repo.CancelPending(r.Context(), id); cancelErr != nil {
			h.writeRepoError(w, cancelErr)
			return
		}
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
	from, to, hasDateRange, dateCode, dateMessage := parseTransactionDateRange(r)
	if dateCode != "" {
		writeAPIError(w, http.StatusBadRequest, dateCode, dateMessage)
		return
	}
	if hasDateRange {
		filter["occurred_at"] = bson.M{"$gte": from, "$lte": to}
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
	from, to, hasDateRange, dateCode, dateMessage := parseTransactionDateRange(r)
	if dateCode != "" {
		writeAPIError(w, http.StatusBadRequest, dateCode, dateMessage)
		return
	}
	month := r.URL.Query().Get("month")
	if !hasDateRange {
		parsed, err := time.Parse("2006-01", month)
		if err != nil || parsed.Format("2006-01") != month {
			writeAPIError(w, http.StatusBadRequest, "INVALID_MONTH", "month must use YYYY-MM")
			return
		}
		from = time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, transactionLocation)
		to = from.AddDate(0, 1, 0)
	}
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
	summaryTo := to
	if hasDateRange {
		summaryTo = to.Add(time.Nanosecond)
	}
	in, out, err := h.repo.Summary(r.Context(), accountID, from.UTC(), summaryTo.UTC())
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	response := map[string]any{"totalIn": in, "totalOut": out}
	if hasDateRange {
		response["from"] = from.Format(time.RFC3339Nano)
		response["to"] = to.Format(time.RFC3339Nano)
	} else {
		response["month"] = month
	}
	httpx.WriteJSON(w, http.StatusOK, response)
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
	value = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "đ", "d")
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
