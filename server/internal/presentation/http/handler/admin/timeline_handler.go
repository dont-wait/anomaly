package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	"github.com/dont-wait/anomaly/internal/timeutil"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

const (
	bankingStream       = journal.BankStream
	auditAccountPrefix  = "audit-account-"
	maxTimelineScan     = 10000
	defaultTimelinePage = 20
	maxTimelinePage     = 20
	timelineCursorV2    = 2
)

type accountTimelineLookup interface {
	FindAll(context.Context) ([]*accountdomain.UserAccount, error)
}

type TimelineHandler struct {
	client   *esdb.Client
	accounts accountTimelineLookup
}

func NewTimelineHandler(client *esdb.Client, accounts accountTimelineLookup) *TimelineHandler {
	return &TimelineHandler{client: client, accounts: accounts}
}

func RegisterTimelineRoutes(router *openapi.Registry, handler *TimelineHandler, tokenSvc queries.TokenService, currentAdmin func(http.Handler) http.Handler) {
	secured := middleware.RequireAuth(tokenSvc)(currentAdmin(http.HandlerFunc(handler.Events)))
	router.Handle("GET /api/admin/audit/accounts/{id}/events", secured, openapi.Operation{
		ID: "adminAccountTimeline", Summary: "Get account audit timeline", Tags: []string{"Admin Audit"}, Auth: true,
		Parameters: []openapi.Parameter{
			{Name: "id", In: "path", Type: "string", Required: true},
			{Name: "from", In: "query", Type: "string", Example: "2026-10-01"},
			{Name: "to", In: "query", Type: "string", Example: "2026-10-30"},
			{Name: "category", In: "query", Type: "string", Example: "all"},
			{Name: "limit", In: "query", Type: "integer", Example: "20"},
			{Name: "cursor", In: "query", Type: "string"},
		},
		Response: timelineResponse{}, FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusInternalServerError},
	})
}

type timelineCursor struct {
	Version         int    `json:"version"`
	AccountID       string `json:"accountId"`
	Category        string `json:"category"`
	From            string `json:"from"`
	To              string `json:"to"`
	BankingRevision *int64 `json:"bankingRevision,omitempty"`
	AuditRevision   *int64 `json:"auditRevision,omitempty"`
}

type timelineAccountDTO struct {
	ID        string `json:"id"`
	AccountNo string `json:"accountNo"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Customer  string `json:"customerName"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	Balance   int64  `json:"balance"`
}

type timelineItemDTO struct {
	EventID        string         `json:"eventId"`
	EventType      string         `json:"eventType"`
	Category       string         `json:"category"`
	OccurredAt     time.Time      `json:"occurredAt"`
	SourceStream   string         `json:"sourceStream"`
	SourceRevision uint64         `json:"sourceRevision"`
	AccountID      string         `json:"accountId,omitempty"`
	ActorID        string         `json:"actorId,omitempty"`
	TransactionID  string         `json:"transactionId,omitempty"`
	Summary        string         `json:"summary"`
	Details        map[string]any `json:"details,omitempty"`
}

type timelineResponse struct {
	Account    timelineAccountDTO `json:"account"`
	Items      []timelineItemDTO  `json:"items"`
	NextCursor string             `json:"nextCursor,omitempty"`
	HasMore    bool               `json:"hasMore"`
}

func (h *TimelineHandler) Events(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	account, err := h.resolveAccount(ctx, r.PathValue("id"))
	if errors.Is(err, errTimelineAccountNotFound) {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not resolve account", http.StatusInternalServerError)
		return
	}

	from, to, err := parseTimelineRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
	toRaw := strings.TrimSpace(r.URL.Query().Get("to"))
	category := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category")))
	if category == "" {
		category = "all"
	}
	if !validTimelineCategory(category) {
		http.Error(w, "category must be all, account, transfer, otp, or login", http.StatusBadRequest)
		return
	}
	limit := parseTimelineLimit(r.URL.Query().Get("limit"))
	cursor, err := decodeTimelineCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("cursor")) != "" {
		if err := validateTimelineCursor(cursor, account.Id, category, fromRaw, toRaw); err != nil {
			http.Error(w, "cursor does not match the requested timeline", http.StatusBadRequest)
			return
		}
	}

	items, next, hasMore, err := h.loadTimelinePage(ctx, account, from, to, fromRaw, toRaw, category, cursor, limit)
	if err != nil {
		if errors.Is(err, errTimelineTooLarge) {
			http.Error(w, "history is too large; narrow the date range", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "could not load account history", http.StatusInternalServerError)
		return
	}
	response := timelineResponse{
		Account:    accountDTO(account),
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(response)
}

var (
	errTimelineAccountNotFound = errors.New("timeline account not found")
	errTimelineTooLarge        = errors.New("timeline scan too large")
)

func (h *TimelineHandler) resolveAccount(ctx context.Context, query string) (*accountdomain.UserAccount, error) {
	if h.accounts == nil {
		return nil, errTimelineAccountNotFound
	}
	accounts, err := h.accounts.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if strings.EqualFold(query, account.Id) || strings.EqualFold(query, account.AccountNo) || strings.EqualFold(query, account.Username) || strings.EqualFold(query, account.Email) {
			return account, nil
		}
	}
	return nil, errTimelineAccountNotFound
}

func accountDTO(account *accountdomain.UserAccount) timelineAccountDTO {
	name := ""
	if account.Customer != nil {
		name = account.Customer.Profile.FullName
	}
	return timelineAccountDTO{ID: account.Id, AccountNo: account.AccountNo, Username: account.Username, Email: account.Email, Customer: name, Currency: string(account.Currency), Status: string(account.Status), Balance: account.Balance.Current}
}

type timelineCandidate struct {
	item     timelineItemDTO
	revision int64
	next     int64
	source   string
}

func (h *TimelineHandler) loadTimelinePage(ctx context.Context, account *accountdomain.UserAccount, from, to time.Time, fromRaw, toRaw, category string, cursor timelineCursor, limit int) ([]timelineItemDTO, string, bool, error) {
	financialID := journal.FinancialAccountID(account.Id)
	positions := cursor
	budget := maxTimelineScan
	var bankingCandidate, auditCandidate *timelineCandidate
	items := make([]timelineItemDTO, 0, limit)

	for len(items) < limit {
		var err error
		if bankingCandidate == nil {
			bankingCandidate, positions.BankingRevision, err = h.nextBankingCandidate(ctx, account.Id, financialID, from, to, category, positions.BankingRevision, &budget)
			if err != nil {
				return nil, "", false, err
			}
		}
		if auditCandidate == nil {
			auditCandidate, positions.AuditRevision, err = h.nextAuditCandidate(ctx, from, to, category, positions.AuditRevision, &budget, account.Id)
			if err != nil {
				return nil, "", false, err
			}
		}
		if bankingCandidate == nil && auditCandidate == nil {
			break
		}
		selected := selectTimelineCandidate(bankingCandidate, auditCandidate)
		if selected == bankingCandidate {
			items = append(items, bankingCandidate.item)
			positions.BankingRevision = int64Pointer(bankingCandidate.next)
			bankingCandidate = nil
		} else {
			items = append(items, auditCandidate.item)
			positions.AuditRevision = int64Pointer(auditCandidate.next)
			auditCandidate = nil
		}
	}

	if bankingCandidate == nil {
		var err error
		bankingCandidate, positions.BankingRevision, err = h.nextBankingCandidate(ctx, account.Id, financialID, from, to, category, positions.BankingRevision, &budget)
		if err != nil {
			return nil, "", false, err
		}
	}
	if auditCandidate == nil {
		var err error
		auditCandidate, positions.AuditRevision, err = h.nextAuditCandidate(ctx, from, to, category, positions.AuditRevision, &budget, account.Id)
		if err != nil {
			return nil, "", false, err
		}
	}
	hasMore := bankingCandidate != nil || auditCandidate != nil
	if !hasMore {
		return items, "", false, nil
	}
	positions.Version = timelineCursorV2
	positions.AccountID = account.Id
	positions.Category = category
	positions.From = fromRaw
	positions.To = toRaw
	return items, encodeTimelineCursor(positions), true, nil
}

func timelineCandidateAfter(left, right *timelineCandidate) bool {
	if left.item.OccurredAt.Equal(right.item.OccurredAt) {
		return timelineKey(left.item) > timelineKey(right.item)
	}
	return left.item.OccurredAt.After(right.item.OccurredAt)
}

func selectTimelineCandidate(banking, audit *timelineCandidate) *timelineCandidate {
	if banking == nil {
		return audit
	}
	if audit == nil || timelineCandidateAfter(banking, audit) {
		return banking
	}
	return audit
}

func int64Pointer(value int64) *int64 { return &value }

func (h *TimelineHandler) nextBankingCandidate(ctx context.Context, accountID, financialID string, from, to time.Time, category string, position *int64, budget *int) (*timelineCandidate, *int64, error) {
	for *budget > 0 {
		(*budget)--
		recorded, found, err := readOneTimelineEvent(ctx, h.client, bankingStream, position)
		if err != nil {
			return nil, position, err
		}
		if !found {
			return nil, int64Pointer(-1), nil
		}
		item, ok, err := bankingTimelineItem(recorded, accountID, financialID)
		if err != nil {
			return nil, position, err
		}
		next := int64(recorded.Revision) - 1
		if ok && timelineMatches(item, from, to, category) {
			return &timelineCandidate{item: item, revision: int64(recorded.Revision), next: next, source: bankingStream}, int64Pointer(int64(recorded.Revision)), nil
		}
		position = int64Pointer(next)
	}
	return nil, position, errTimelineTooLarge
}

func (h *TimelineHandler) nextAuditCandidate(ctx context.Context, from, to time.Time, category string, position *int64, budget *int, accountID string) (*timelineCandidate, *int64, error) {
	streamID := auditAccountPrefix + accountID
	for *budget > 0 {
		(*budget)--
		recorded, found, err := readOneTimelineEvent(ctx, h.client, streamID, position)
		if err != nil {
			if resourceMissingTimeline(err) {
				return nil, int64Pointer(-1), nil
			}
			return nil, position, err
		}
		if !found {
			return nil, int64Pointer(-1), nil
		}
		var event audit.Event
		if err := json.Unmarshal(recorded.Data, &event); err != nil {
			return nil, position, err
		}
		item, ok := auditTimelineItem(recorded, event)
		next := int64(recorded.Revision) - 1
		if ok && timelineMatches(item, from, to, category) {
			return &timelineCandidate{item: item, revision: int64(recorded.Revision), next: next, source: streamID}, int64Pointer(int64(recorded.Revision)), nil
		}
		position = int64Pointer(next)
	}
	return nil, position, errTimelineTooLarge
}

type timelineRecorded struct {
	StreamID string
	EventID  string
	Type     string
	Revision uint64
	Data     []byte
	Created  time.Time
}

func readOneTimelineEvent(ctx context.Context, client *esdb.Client, streamID string, position *int64) (timelineRecorded, bool, error) {
	if client == nil {
		return timelineRecorded{}, false, errors.New("event store is unavailable")
	}
	if position != nil && *position < 0 {
		return timelineRecorded{}, false, nil
	}
	from := esdb.StreamPosition(esdb.End{})
	if position != nil {
		from = esdb.Revision(uint64(*position))
	}
	stream, err := client.ReadStream(ctx, streamID, esdb.ReadStreamOptions{Direction: esdb.Backwards, From: from}, 1)
	if err != nil {
		return timelineRecorded{}, false, err
	}
	defer stream.Close()
	event, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		return timelineRecorded{}, false, nil
	}
	if err != nil {
		return timelineRecorded{}, false, err
	}
	return timelineRecorded{StreamID: streamID, EventID: event.Event.EventID.String(), Type: event.Event.EventType, Revision: event.Event.EventNumber, Data: append([]byte(nil), event.Event.Data...), Created: event.Event.CreatedDate}, true, nil
}

func resourceMissingTimeline(err error) bool {
	var eventStoreErr *esdb.Error
	return errors.As(err, &eventStoreErr) && eventStoreErr.Code() == esdb.ErrorCodeResourceNotFound
}

func bankingTimelineItem(recorded timelineRecorded, accountID, financialID string) (timelineItemDTO, bool, error) {
	var record journal.Record
	if err := json.Unmarshal(recorded.Data, &record); err != nil {
		return timelineItemDTO{}, false, err
	}
	if record.ID != recorded.EventID || record.Type != recorded.Type {
		return timelineItemDTO{}, false, fmt.Errorf("banking event metadata mismatch")
	}
	item := timelineItemDTO{EventID: recorded.EventID, EventType: recorded.Type, SourceStream: bankingStream, SourceRevision: recorded.Revision, AccountID: accountID, OccurredAt: record.At, Details: map[string]any{}}
	switch record.Type {
	case journal.AccountCreated:
		if record.Account == nil || record.Account.Id != accountID {
			return timelineItemDTO{}, false, nil
		}
		item.Category, item.Summary = "account", "Tài khoản được tạo"
	case journal.AccountUpdated:
		if record.Account == nil || record.Account.Id != accountID {
			return timelineItemDTO{}, false, nil
		}
		item.Category, item.Summary = "account", "Tài khoản được cập nhật"
	case journal.BalanceSeeded:
		if journal.FinancialAccountID(record.AccountID) != financialID {
			return timelineItemDTO{}, false, nil
		}
		item.Category, item.Summary = "account", "Số dư được thiết lập"
		item.Details["balance"] = record.Balance
	case journal.BankImported:
		matched := false
		for _, account := range record.Accounts {
			if account != nil && account.Id == accountID {
				matched = true
				break
			}
		}
		if !matched {
			return timelineItemDTO{}, false, nil
		}
		item.Category, item.Summary = "account", "Dữ liệu tài khoản được nhập"
	default:
		if record.Event == nil || !journal.IsTransferEvent(record.Type) {
			return timelineItemDTO{}, false, nil
		}
		payload := record.Event.Payload
		isSource := payload.SourceAccountID == financialID
		isDestination := payload.DestinationAccountID == financialID
		if record.Type == journal.Created || record.Type == journal.OTPUpdated || record.Type == journal.Failed || record.Type == journal.Cancelled {
			if !isSource {
				return timelineItemDTO{}, false, nil
			}
		} else if !isSource && !isDestination {
			return timelineItemDTO{}, false, nil
		}
		item.TransactionID = record.Event.TransactionID
		item.OccurredAt = record.Event.OccurredAt
		item.Details["amount"] = payload.Amount
		item.Details["fee"] = payload.Fee
		item.Details["currency"] = payload.Currency
		item.Details["status"] = payload.Status
		if isSource {
			item.Details["direction"] = "out"
		} else {
			item.Details["direction"] = "in"
		}
		if payload.FailureReason != "" {
			item.Details["failureReason"] = payload.FailureReason
		}
		if payload.CancellationReason != "" {
			item.Details["cancellationReason"] = payload.CancellationReason
		}
		if payload.SourceBalanceBefore != nil && isSource {
			item.Details["balanceBefore"] = *payload.SourceBalanceBefore
			item.Details["balanceAfter"] = payload.SourceBalanceAfter
		}
		if payload.DestinationBalanceBefore != nil && isDestination {
			item.Details["balanceBefore"] = *payload.DestinationBalanceBefore
			item.Details["balanceAfter"] = payload.DestinationBalanceAfter
		}
		if record.Type == journal.OTPUpdated {
			item.Category, item.Summary = "otp", "OTP chuyển tiền được cập nhật"
		} else {
			item.Category, item.Summary = "transfer", transferSummary(record.Type, isSource)
		}
	}
	if item.OccurredAt.IsZero() {
		item.OccurredAt = recorded.Created
	}
	return item, item.Summary != "", nil
}

func auditTimelineItem(recorded timelineRecorded, event audit.Event) (timelineItemDTO, bool) {
	category := ""
	summary := ""
	switch {
	case strings.HasPrefix(event.Type, "TransferOTP"):
		category, summary = "otp", "OTP chuyển tiền được xác thực"
	case strings.HasPrefix(event.Type, "EmailOTP"):
		category, summary = "otp", "OTP email được xác thực"
	case strings.HasPrefix(event.Type, "Login"):
		category, summary = "login", "Đăng nhập"
	default:
		return timelineItemDTO{}, false
	}
	item := timelineItemDTO{EventID: event.ID, EventType: event.Type, Category: category, OccurredAt: event.OccurredAt, SourceStream: recorded.StreamID, SourceRevision: recorded.Revision, AccountID: event.AccountID, ActorID: event.ActorID, TransactionID: event.TransactionID, Summary: summary, Details: map[string]any{}}
	if event.ReasonCode != "" {
		item.Details["reasonCode"] = event.ReasonCode
	}
	if event.AttemptsLeft != nil {
		item.Details["attemptsLeft"] = *event.AttemptsLeft
	}
	return item, true
}

func transferSummary(eventType string, source bool) string {
	direction := "Nhận tiền"
	if source {
		direction = "Chuyển tiền"
	}
	switch eventType {
	case journal.Created:
		return direction + " được tạo"
	case journal.Completed:
		return direction + " thành công"
	case journal.Failed:
		return direction + " thất bại"
	case journal.Cancelled:
		return direction + " bị hủy"
	default:
		return direction
	}
}

func timelineMatches(item timelineItemDTO, from, to time.Time, category string) bool {
	if category != "all" && item.Category != category {
		return false
	}
	if !from.IsZero() && item.OccurredAt.Before(from) {
		return false
	}
	return to.IsZero() || item.OccurredAt.Before(to)
}

func validTimelineCategory(category string) bool {
	switch category {
	case "all", "account", "transfer", "otp", "login":
		return true
	default:
		return false
	}
}

func parseTimelineRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	if fromRaw == "" && toRaw == "" {
		return time.Time{}, time.Time{}, nil
	}
	if fromRaw == "" || toRaw == "" {
		return time.Time{}, time.Time{}, errors.New("from and to are required together")
	}
	fromDate, err := time.ParseInLocation("2006-01-02", fromRaw, timeutil.VietnamLocation)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("from and to must use YYYY-MM-DD")
	}
	toDate, err := time.ParseInLocation("2006-01-02", toRaw, timeutil.VietnamLocation)
	if err != nil || toDate.Before(fromDate) {
		return time.Time{}, time.Time{}, errors.New("invalid date range")
	}
	return fromDate.UTC(), toDate.AddDate(0, 0, 1).UTC(), nil
}

func parseTimelineLimit(value string) int {
	var limit int
	if _, err := fmt.Sscan(value, &limit); err != nil || limit <= 0 {
		return defaultTimelinePage
	}
	if limit > maxTimelinePage {
		return maxTimelinePage
	}
	return limit
}

func encodeTimelineCursor(cursor timelineCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeTimelineCursor(value string) (timelineCursor, error) {
	if strings.TrimSpace(value) == "" {
		return timelineCursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return timelineCursor{}, err
	}
	var cursor timelineCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != timelineCursorV2 || cursor.AccountID == "" || cursor.Category == "" || cursor.BankingRevision == nil || cursor.AuditRevision == nil || invalidTimelineRevision(cursor.BankingRevision) || invalidTimelineRevision(cursor.AuditRevision) {
		return timelineCursor{}, errors.New("invalid cursor")
	}
	return cursor, nil
}

func invalidTimelineRevision(revision *int64) bool {
	return revision == nil || *revision < -1
}

func validateTimelineCursor(cursor timelineCursor, accountID, category, from, to string) error {
	if cursor.AccountID != accountID || cursor.Category != category || cursor.From != from || cursor.To != to {
		return errors.New("cursor does not match timeline query")
	}
	return nil
}

func timelineKey(item timelineItemDTO) string {
	return fmt.Sprintf("%s:%020d:%s", item.SourceStream, item.SourceRevision, item.EventID)
}
