package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	defaultTimelinePage = 50
	maxTimelinePage     = 200
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
			{Name: "limit", In: "query", Type: "integer", Example: "50"},
			{Name: "cursor", In: "query", Type: "string"},
		},
		Response: timelineResponse{}, FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusInternalServerError},
	})
}

type timelineCursor struct {
	OccurredAt time.Time `json:"occurredAt"`
	Key        string    `json:"key"`
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

type timelineEvent struct {
	Item timelineItemDTO
	Key  string
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

	items, err := h.loadTimeline(ctx, account, from, to, category)
	if err != nil {
		if errors.Is(err, errTimelineTooLarge) {
			http.Error(w, "history is too large; narrow the date range", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "could not load account history", http.StatusInternalServerError)
		return
	}
	page, next, hasMore := paginateTimeline(items, cursor, limit)
	response := timelineResponse{
		Account:    accountDTO(account),
		Items:      page,
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

func (h *TimelineHandler) loadTimeline(ctx context.Context, account *accountdomain.UserAccount, from, to time.Time, category string) ([]timelineEvent, error) {
	financialID := journal.FinancialAccountID(account.Id)
	items := make([]timelineEvent, 0)
	banking, err := readTimelineStream(ctx, h.client, bankingStream, maxTimelineScan)
	if err != nil {
		return nil, err
	}
	for _, recorded := range banking {
		item, ok, decodeErr := bankingTimelineItem(recorded, account.Id, financialID)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if ok && timelineMatches(item, from, to, category) {
			items = append(items, timelineEvent{Item: item, Key: timelineKey(item)})
		}
	}

	auditEvents, err := readTimelineStream(ctx, h.client, auditAccountPrefix+account.Id, maxTimelineScan)
	if err != nil && !resourceMissingTimeline(err) {
		return nil, err
	}
	for _, recorded := range auditEvents {
		var event audit.Event
		if err := json.Unmarshal(recorded.Data, &event); err != nil {
			return nil, err
		}
		item, ok := auditTimelineItem(recorded, event)
		if ok && timelineMatches(item, from, to, category) {
			items = append(items, timelineEvent{Item: item, Key: timelineKey(item)})
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Item.OccurredAt.Equal(items[j].Item.OccurredAt) {
			return items[i].Key > items[j].Key
		}
		return items[i].Item.OccurredAt.After(items[j].Item.OccurredAt)
	})
	return items, nil
}

type timelineRecorded struct {
	StreamID string
	EventID  string
	Type     string
	Revision uint64
	Data     []byte
	Created  time.Time
}

func readTimelineStream(ctx context.Context, client *esdb.Client, streamID string, max int) ([]timelineRecorded, error) {
	if client == nil {
		return nil, errors.New("event store is unavailable")
	}
	const pageSize = 4096
	result := make([]timelineRecorded, 0)
	opts := esdb.ReadStreamOptions{From: esdb.Start{}}
	for {
		stream, err := client.ReadStream(ctx, streamID, opts, pageSize)
		if err != nil {
			return nil, err
		}
		count := 0
		var last uint64
		for {
			event, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				if resourceMissingTimeline(err) && count == 0 {
					stream.Close()
					return result, nil
				}
				stream.Close()
				return nil, err
			}
			count++
			last = event.Event.EventNumber
			if len(result) >= max {
				stream.Close()
				return nil, errTimelineTooLarge
			}
			result = append(result, timelineRecorded{StreamID: streamID, EventID: event.Event.EventID.String(), Type: event.Event.EventType, Revision: event.Event.EventNumber, Data: append([]byte(nil), event.Event.Data...), Created: event.Event.CreatedDate})
		}
		stream.Close()
		if count < pageSize {
			return result, nil
		}
		opts = esdb.ReadStreamOptions{From: esdb.Revision(last + 1)}
	}
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
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.OccurredAt.IsZero() || cursor.Key == "" {
		return timelineCursor{}, errors.New("invalid cursor")
	}
	return cursor, nil
}

func timelineKey(item timelineItemDTO) string {
	return fmt.Sprintf("%s:%020d:%s", item.SourceStream, item.SourceRevision, item.EventID)
}

func paginateTimeline(items []timelineEvent, cursor timelineCursor, limit int) ([]timelineItemDTO, string, bool) {
	start := 0
	if !cursor.OccurredAt.IsZero() {
		for start < len(items) {
			item := items[start]
			if item.Item.OccurredAt.Before(cursor.OccurredAt) || (item.Item.OccurredAt.Equal(cursor.OccurredAt) && item.Key < cursor.Key) {
				break
			}
			start++
		}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := make([]timelineItemDTO, 0, end-start)
	for _, item := range items[start:end] {
		page = append(page, item.Item)
	}
	hasMore := end < len(items)
	if !hasMore || len(page) == 0 {
		return page, "", hasMore
	}
	last := items[end-1]
	return page, encodeTimelineCursor(timelineCursor{OccurredAt: last.Item.OccurredAt, Key: last.Key}), true
}
