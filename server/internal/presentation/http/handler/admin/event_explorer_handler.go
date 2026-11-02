package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

const (
	eventExplorerCursorVersion = 1
	eventExplorerDefaultLimit  = 20
	eventExplorerMaxLimit      = 50
	eventExplorerBatchSize     = 256
	eventExplorerMaxScan       = 5000
	eventExplorerAuditGlobal   = "audit-global"
	eventExplorerAccountPrefix = "account-"
)

type eventExplorerCursor struct {
	Version  int    `json:"version"`
	Commit   uint64 `json:"commit"`
	Prepare  uint64 `json:"prepare"`
	QueryKey string `json:"queryKey"`
}

type eventExplorerItemDTO struct {
	EventID        string                    `json:"eventId"`
	EventType      string                    `json:"eventType"`
	Category       string                    `json:"category"`
	OccurredAt     time.Time                 `json:"occurredAt"`
	SourceStream   string                    `json:"sourceStream"`
	SourceRevision uint64                    `json:"sourceRevision"`
	AccountIDs     []string                  `json:"-"`
	AccountRoles   map[string]string         `json:"-"`
	Accounts       []eventExplorerAccountDTO `json:"accounts,omitempty"`
	ActorID        string                    `json:"actorId,omitempty"`
	TransactionID  string                    `json:"transactionId,omitempty"`
	LoanID         string                    `json:"loanId,omitempty"`
	Summary        string                    `json:"summary"`
	Details        map[string]any            `json:"details,omitempty"`
}

type eventExplorerAccountDTO struct {
	AccountNo    string `json:"accountNo"`
	CustomerName string `json:"customerName,omitempty"`
	Username     string `json:"username,omitempty"`
	Role         string `json:"role,omitempty"`
}

type eventExplorerResponse struct {
	Items        []eventExplorerListItemDTO `json:"items"`
	NextCursor   string                     `json:"nextCursor,omitempty"`
	HasMore      bool                       `json:"hasMore"`
	ScanComplete bool                       `json:"scanComplete"`
	Scanned      int                        `json:"scanned"`
}

type eventExplorerListItemDTO struct {
	EventID       string                    `json:"eventId"`
	Category      string                    `json:"category"`
	OccurredAt    time.Time                 `json:"occurredAt"`
	Accounts      []eventExplorerAccountDTO `json:"accounts,omitempty"`
	TransactionID string                    `json:"transactionId,omitempty"`
	LoanID        string                    `json:"loanId,omitempty"`
	Summary       string                    `json:"summary"`
}

type eventExplorerQuery struct {
	EventID       string
	TransactionID string
	AccountRaw    string
	AccountID     string
	Stream        string
	Category      string
	EventType     string
	Status        string
	From          time.Time
	To            time.Time
	FromRaw       string
	ToRaw         string
	Limit         int
}

func RegisterEventExplorerRoutes(router *openapi.Registry, handler *TimelineHandler, tokenSvc queries.TokenService, currentAdmin func(http.Handler) http.Handler) {
	secured := middleware.RequireAuth(tokenSvc)(currentAdmin(http.HandlerFunc(handler.EventExplorer)))
	securedDetail := middleware.RequireAuth(tokenSvc)(currentAdmin(http.HandlerFunc(handler.EventExplorerDetail)))
	router.Handle("GET /api/admin/audit/events", secured, openapi.Operation{
		ID: "adminEventExplorer", Summary: "Search admin audit events", Tags: []string{"Admin Audit"}, Auth: true,
		Parameters: []openapi.Parameter{
			{Name: "transactionId", In: "query", Type: "string"},
			{Name: "account", In: "query", Type: "string", Example: "0123456789"},
			{Name: "category", In: "query", Type: "string", Example: "transfer"},
			{Name: "eventType", In: "query", Type: "string"},
			{Name: "status", In: "query", Type: "string"},
			{Name: "from", In: "query", Type: "string", Example: "2026-10-01"},
			{Name: "to", In: "query", Type: "string", Example: "2026-10-30"},
			{Name: "limit", In: "query", Type: "integer", Example: "20"},
			{Name: "cursor", In: "query", Type: "string"},
		},
		Response: eventExplorerResponse{}, FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusInternalServerError},
	})
	router.Handle("GET /api/admin/audit/events/{id}", securedDetail, openapi.Operation{
		ID: "adminEventExplorerDetail", Summary: "Get admin audit event detail", Tags: []string{"Admin Audit"}, Auth: true,
		Parameters: []openapi.Parameter{
			{Name: "id", In: "path", Type: "string", Required: true},
			{Name: "stream", In: "query", Type: "string"},
			{Name: "revision", In: "query", Type: "integer"},
		},
		Response: eventExplorerItemDTO{}, FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusInternalServerError},
	})
}

func (h *TimelineHandler) EventExplorer(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query, err := parseEventExplorerQuery(r)
	if err != nil {
		writeEventExplorerError(w, http.StatusBadRequest, err.Error())
		return
	}
	var accountDirectory []*accountdomain.UserAccount
	if query.AccountRaw != "" {
		accountDirectory, err = h.loadEventExplorerAccounts(ctx)
		if err != nil {
			writeEventExplorerError(w, http.StatusInternalServerError, "could not resolve account")
			return
		}
		account := findEventExplorerAccount(accountDirectory, query.AccountRaw)
		if account == nil {
			writeEventExplorerError(w, http.StatusNotFound, "account not found")
			return
		}
		query.AccountID = account.Id
	}

	queryKey := eventExplorerQueryKey(query)
	cursor, err := decodeEventExplorerCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeEventExplorerError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	if cursor.QueryKey != "" && cursor.QueryKey != queryKey {
		writeEventExplorerError(w, http.StatusBadRequest, "cursor does not match the requested filters")
		return
	}

	items, nextCursor, hasMore, scanned, err := h.readEventExplorerPage(ctx, query, cursor)
	if err != nil {
		if errors.Is(err, errEventExplorerTooLarge) {
			writeEventExplorerError(w, http.StatusRequestEntityTooLarge, "event search is too broad; narrow the filters")
			return
		}
		writeEventExplorerError(w, http.StatusInternalServerError, "could not load audit events")
		return
	}
	if eventExplorerItemsHaveAccounts(items) && accountDirectory == nil {
		accountDirectory, err = h.loadEventExplorerAccounts(ctx)
		if err != nil {
			accountDirectory = nil
		}
	}
	if accountDirectory != nil {
		h.enrichEventExplorerAccounts(items, accountDirectory)
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	httpx.WriteSuccess(w, http.StatusOK, "Audit events loaded", eventExplorerResponse{
		Items:        eventExplorerListItems(items),
		NextCursor:   nextCursor,
		HasMore:      hasMore,
		ScanComplete: !hasMore,
		Scanned:      scanned,
	})
}

func (h *TimelineHandler) EventExplorerDetail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	eventID := strings.TrimSpace(r.PathValue("id"))
	if eventID == "" {
		writeEventExplorerError(w, http.StatusBadRequest, "event id is required")
		return
	}
	streamID := strings.TrimSpace(r.URL.Query().Get("stream"))
	rawRevision := strings.TrimSpace(r.URL.Query().Get("revision"))
	var item eventExplorerItemDTO
	var found bool
	var err error
	if streamID != "" || rawRevision != "" {
		if streamID == "" || rawRevision == "" {
			writeEventExplorerError(w, http.StatusBadRequest, "stream and revision must be provided together")
			return
		}
		revision, parseErr := strconv.ParseUint(rawRevision, 10, 64)
		if parseErr != nil {
			writeEventExplorerError(w, http.StatusBadRequest, "revision must be a positive integer")
			return
		}
		item, found, err = h.readEventExplorerDetail(ctx, eventID, streamID, revision)
	} else {
		item, found, err = h.findEventExplorerItem(ctx, eventID)
	}
	if err != nil {
		if errors.Is(err, errEventExplorerTooLarge) {
			writeEventExplorerError(w, http.StatusRequestEntityTooLarge, "event search is too broad; provide stream and revision")
			return
		}
		writeEventExplorerError(w, http.StatusInternalServerError, "could not load audit event")
		return
	}
	if !found {
		writeEventExplorerError(w, http.StatusNotFound, "event not found")
		return
	}
	enrichedItems := []eventExplorerItemDTO{item}
	if eventExplorerItemsHaveAccounts(enrichedItems) {
		accountDirectory, loadErr := h.loadEventExplorerAccounts(ctx)
		if loadErr == nil {
			h.enrichEventExplorerAccounts(enrichedItems, accountDirectory)
		}
	}
	item = enrichedItems[0]
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	httpx.WriteSuccess(w, http.StatusOK, "Audit event loaded", item)
}

func writeEventExplorerError(w http.ResponseWriter, status int, detail string) {
	httpx.WriteJSON(w, status, httpx.ErrorResponse{
		Status: status,
		Title:  http.StatusText(status),
		Errors: []httpx.ErrorDetail{{Code: httpx.ErrorCodeForStatus(status), Detail: detail}},
	})
}

func (h *TimelineHandler) findEventExplorerItem(ctx context.Context, eventID string) (eventExplorerItemDTO, bool, error) {
	query := eventExplorerQuery{EventID: eventID, Category: "all", Limit: 1}
	cursor := eventExplorerCursor{}
	scanned := 0
	for {
		items, nextCursor, hasMore, pageScanned, err := h.readEventExplorerPage(ctx, query, cursor)
		if err != nil {
			return eventExplorerItemDTO{}, false, err
		}
		scanned += pageScanned
		if len(items) > 0 {
			return items[0], true, nil
		}
		if !hasMore || nextCursor == "" {
			return eventExplorerItemDTO{}, false, nil
		}
		if scanned >= eventExplorerMaxScan {
			return eventExplorerItemDTO{}, false, errEventExplorerTooLarge
		}
		cursor, err = decodeEventExplorerCursor(nextCursor)
		if err != nil {
			return eventExplorerItemDTO{}, false, err
		}
	}
}

var errEventExplorerTooLarge = errors.New("event explorer scan too large")

func parseEventExplorerQuery(r *http.Request) (eventExplorerQuery, error) {
	values := r.URL.Query()
	query := eventExplorerQuery{
		EventID:       strings.TrimSpace(values.Get("eventId")),
		TransactionID: strings.TrimSpace(values.Get("transactionId")),
		AccountRaw:    strings.TrimSpace(values.Get("account")),
		Stream:        strings.TrimSpace(values.Get("stream")),
		Category:      strings.ToLower(strings.TrimSpace(values.Get("category"))),
		EventType:     strings.TrimSpace(values.Get("eventType")),
		Status:        strings.TrimSpace(values.Get("status")),
		FromRaw:       strings.TrimSpace(values.Get("from")),
		ToRaw:         strings.TrimSpace(values.Get("to")),
		Limit:         eventExplorerDefaultLimit,
	}
	if query.Category == "" {
		query.Category = "all"
	}
	if !validEventExplorerCategory(query.Category) {
		return eventExplorerQuery{}, errors.New("category must be all, account, transfer, otp, login, loan, or other")
	}
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 {
			return eventExplorerQuery{}, errors.New("limit must be a positive integer")
		}
		if limit > eventExplorerMaxLimit {
			limit = eventExplorerMaxLimit
		}
		query.Limit = limit
	}
	var err error
	query.From, query.To, err = parseEventExplorerRange(query.FromRaw, query.ToRaw)
	if err != nil {
		return eventExplorerQuery{}, err
	}
	return query, nil
}

func parseEventExplorerRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	var from, to time.Time
	var err error
	if fromRaw != "" {
		from, err = parseEventExplorerTime(fromRaw, false)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("from must be an RFC3339 timestamp or YYYY-MM-DD")
		}
	}
	if toRaw != "" {
		to, err = parseEventExplorerTime(toRaw, true)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("to must be an RFC3339 timestamp or YYYY-MM-DD")
		}
	}
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return time.Time{}, time.Time{}, errors.New("from must be before to")
	}
	return from, to, nil
}

func parseEventExplorerTime(raw string, endOfDay bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		return parsed.UTC().Add(24 * time.Hour), nil
	}
	return parsed.UTC(), nil
}

func validEventExplorerCategory(category string) bool {
	switch category {
	case "all", "account", "transfer", "otp", "login", "loan", "other":
		return true
	default:
		return false
	}
}

func eventExplorerQueryKey(query eventExplorerQuery) string {
	return strings.Join([]string{query.EventID, query.TransactionID, query.AccountRaw, query.AccountID, query.Stream, query.Category, query.EventType, query.Status, query.FromRaw, query.ToRaw, strconv.Itoa(query.Limit)}, "\x00")
}

func encodeEventExplorerCursor(cursor eventExplorerCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeEventExplorerCursor(raw string) (eventExplorerCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return eventExplorerCursor{}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return eventExplorerCursor{}, errors.New("invalid cursor")
	}
	var cursor eventExplorerCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != eventExplorerCursorVersion || cursor.Commit == 0 || cursor.Prepare == 0 {
		return eventExplorerCursor{}, errors.New("invalid cursor")
	}
	return cursor, nil
}

func (h *TimelineHandler) readEventExplorerPage(ctx context.Context, query eventExplorerQuery, cursor eventExplorerCursor) ([]eventExplorerItemDTO, string, bool, int, error) {
	if h == nil || h.client == nil {
		return nil, "", false, 0, errors.New("event store is unavailable")
	}
	opts := esdb.ReadAllOptions{Direction: esdb.Backwards, From: esdb.End{}}
	if cursor.Commit != 0 {
		opts.From = esdb.Position{Commit: cursor.Commit, Prepare: cursor.Prepare}
	}
	stream, err := h.client.ReadAll(ctx, opts, eventExplorerBatchSize+1)
	if err != nil {
		return nil, "", false, 0, err
	}
	defer stream.Close()

	items := make([]eventExplorerItemDTO, 0, query.Limit)
	scanned := 0
	readCount := 0
	sourceExhausted := false
	pageFull := false
	var lastScannedPosition esdb.Position
	var lastReturnedPosition esdb.Position
	for scanned < eventExplorerMaxScan && readCount < eventExplorerBatchSize+1 {
		recorded, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			sourceExhausted = true
			break
		}
		if recvErr != nil {
			return nil, "", false, scanned, recvErr
		}
		readCount++
		if recorded == nil || recorded.Event == nil {
			continue
		}
		position := recorded.Event.Position
		if cursor.Commit != 0 && position.Commit == cursor.Commit && position.Prepare == cursor.Prepare {
			continue
		}
		scanned++
		lastScannedPosition = position
		item, ok := buildEventExplorerItem(recorded.Event)
		if !ok || !eventExplorerMatches(item, query) {
			continue
		}
		if len(items) == query.Limit {
			pageFull = true
			break
		}
		items = append(items, item)
		lastReturnedPosition = position
		if len(items) == query.Limit {
			pageFull = true
		}
	}
	hasMore := pageFull && !sourceExhausted
	if !pageFull && !sourceExhausted && (readCount >= eventExplorerBatchSize+1 || scanned >= eventExplorerMaxScan) {
		hasMore = true
	}
	if !hasMore {
		return items, "", false, scanned, nil
	}
	cursorPosition := lastScannedPosition
	if pageFull {
		cursorPosition = lastReturnedPosition
	}
	if cursorPosition == (esdb.Position{}) {
		return items, "", false, scanned, nil
	}
	nextCursor, err := encodeEventExplorerCursor(eventExplorerCursor{
		Version:  eventExplorerCursorVersion,
		Commit:   cursorPosition.Commit,
		Prepare:  cursorPosition.Prepare,
		QueryKey: eventExplorerQueryKey(query),
	})
	if err != nil {
		return nil, "", false, scanned, err
	}
	return items, nextCursor, true, scanned, nil
}

func (h *TimelineHandler) readEventExplorerDetail(ctx context.Context, eventID, streamID string, revision uint64) (eventExplorerItemDTO, bool, error) {
	if h == nil || h.client == nil {
		return eventExplorerItemDTO{}, false, errors.New("event store is unavailable")
	}
	if !eventExplorerSupportedStream(streamID) {
		return eventExplorerItemDTO{}, false, nil
	}
	stream, err := h.client.ReadStream(ctx, streamID, esdb.ReadStreamOptions{From: esdb.Revision(revision)}, 1)
	if err != nil {
		return eventExplorerItemDTO{}, false, err
	}
	defer stream.Close()
	recorded, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		return eventExplorerItemDTO{}, false, nil
	}
	if err != nil || recorded == nil || recorded.Event == nil {
		return eventExplorerItemDTO{}, false, err
	}
	if recorded.Event.EventNumber != revision || recorded.Event.EventID.String() != eventID {
		return eventExplorerItemDTO{}, false, nil
	}
	item, ok := buildEventExplorerItem(recorded.Event)
	return item, ok, nil
}

func buildEventExplorerItem(recorded *esdb.RecordedEvent) (eventExplorerItemDTO, bool) {
	if recorded == nil || !eventExplorerSupportedStream(recorded.StreamID) {
		return eventExplorerItemDTO{}, false
	}
	if recorded.StreamID == journal.BankStream && journal.IsTransferEvent(recorded.EventType) {
		var record journal.Record
		if json.Unmarshal(recorded.Data, &record) != nil || record.Event == nil || record.Event.TransactionID == "" {
			return eventExplorerItemDTO{}, false
		}
		event := record.Event
		occurredAt := record.At
		if occurredAt.IsZero() {
			occurredAt = event.OccurredAt
		}
		accountRoles := map[string]string{
			event.Payload.SourceAccountID:      "Gửi",
			event.Payload.DestinationAccountID: "Nhận",
		}
		return eventExplorerItemDTO{
			EventID:        firstNonEmpty(record.ID, recorded.EventID.String()),
			EventType:      event.Type,
			Category:       "transfer",
			OccurredAt:     occurredAt,
			SourceStream:   recorded.StreamID,
			SourceRevision: recorded.EventNumber,
			AccountIDs:     uniqueStrings(event.Payload.SourceAccountID, event.Payload.DestinationAccountID),
			AccountRoles:   accountRoles,
			TransactionID:  event.TransactionID,
			Summary:        event.Type,
			Details: map[string]any{
				"amount":   event.Payload.Amount,
				"currency": event.Payload.Currency,
				"channel":  event.Payload.Channel,
				"status":   event.Payload.Status,
			},
		}, true
	}
	if recorded.StreamID == journal.BankStream {
		return buildGenericBankItem(recorded)
	}
	if strings.HasPrefix(recorded.StreamID, eventExplorerAccountPrefix) {
		return buildGenericAccountItem(recorded)
	}
	if recorded.StreamID == eventExplorerAuditGlobal || strings.HasPrefix(recorded.StreamID, auditAccountPrefix) {
		var event audit.Event
		if json.Unmarshal(recorded.Data, &event) != nil || event.ID == "" || event.Type == "" {
			return eventExplorerItemDTO{}, false
		}
		category := auditEventCategory(event.Type)
		accountID := event.AccountID
		if accountID == "" && strings.HasPrefix(recorded.StreamID, auditAccountPrefix) {
			accountID = strings.TrimPrefix(recorded.StreamID, auditAccountPrefix)
		}
		return eventExplorerItemDTO{
			EventID:        event.ID,
			EventType:      event.Type,
			Category:       category,
			OccurredAt:     event.OccurredAt,
			SourceStream:   recorded.StreamID,
			SourceRevision: recorded.EventNumber,
			AccountIDs:     uniqueStrings(accountID),
			ActorID:        event.ActorID,
			TransactionID:  event.TransactionID,
			Summary:        event.Type,
			Details:        auditEventDetails(event),
		}, true
	}
	if strings.HasPrefix(recorded.StreamID, "loan-application-") && strings.HasPrefix(recorded.EventType, "Loan") {
		return buildGenericLoanItem(recorded)
	}
	return eventExplorerItemDTO{}, false
}

func buildGenericBankItem(recorded *esdb.RecordedEvent) (eventExplorerItemDTO, bool) {
	var record journal.Record
	if json.Unmarshal(recorded.Data, &record) != nil {
		return eventExplorerItemDTO{}, false
	}
	occurredAt := record.At
	if occurredAt.IsZero() {
		occurredAt = recorded.CreatedDate
	}
	accountIDs := make([]string, 0, 4)
	accountIDs = append(accountIDs, record.AccountID)
	if record.Account != nil {
		accountIDs = append(accountIDs, record.Account.Id)
	}
	for _, account := range record.Accounts {
		if account != nil {
			accountIDs = append(accountIDs, account.Id)
		}
	}
	transactionID := ""
	loanID := record.LoanID
	details := map[string]any{}
	if record.Event != nil {
		transactionID = record.Event.TransactionID
	}
	if record.Savings != nil {
		accountIDs = append(accountIDs, record.Savings.Contract.SourceAccountID)
		transactionID = record.Savings.TransactionID
		loanID = record.Savings.Contract.ID
		details["amount"] = record.Savings.Contract.Principal
		details["currency"] = "VND"
		details["status"] = string(record.Savings.Contract.Status)
		if record.Savings.Contract.Settlement != nil {
			details["amount"] = record.Savings.Contract.Settlement.TotalAmount
			details["interestAmount"] = record.Savings.Contract.Settlement.InterestAmount
		}
	}
	if record.LoanPayment != nil {
		accountIDs = append(accountIDs, record.LoanPayment.AccountID)
		transactionID = record.LoanPayment.TransactionID
		loanID = record.LoanPayment.LoanID
		details["amount"] = record.LoanPayment.Amount
		details["principalAmount"] = record.LoanPayment.PrincipalAmount
		details["interestAmount"] = record.LoanPayment.InterestAmount
		details["currency"] = "VND"
		details["status"] = "success"
	}
	if record.LoanAmount > 0 {
		details["amount"] = record.LoanAmount
		details["currency"] = "VND"
		details["status"] = "success"
	}
	if record.BalanceBefore != nil {
		details["balanceBefore"] = *record.BalanceBefore
	}
	if record.BalanceAfter != nil {
		details["balanceAfter"] = *record.BalanceAfter
	}
	category := "account"
	if strings.HasPrefix(recorded.EventType, "Loan") {
		category = "loan"
	} else if strings.HasPrefix(recorded.EventType, "Savings") {
		category = "other"
	}
	return eventExplorerItemDTO{
		EventID:        firstNonEmpty(record.ID, recorded.EventID.String()),
		EventType:      recorded.EventType,
		Category:       category,
		OccurredAt:     occurredAt,
		SourceStream:   recorded.StreamID,
		SourceRevision: recorded.EventNumber,
		AccountIDs:     uniqueStrings(accountIDs...),
		TransactionID:  transactionID,
		LoanID:         loanID,
		Summary:        recorded.EventType,
		Details:        details,
	}, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildGenericAccountItem(recorded *esdb.RecordedEvent) (eventExplorerItemDTO, bool) {
	accountID := strings.TrimPrefix(recorded.StreamID, eventExplorerAccountPrefix)
	return eventExplorerItemDTO{
		EventID:        recorded.EventID.String(),
		EventType:      recorded.EventType,
		Category:       "account",
		OccurredAt:     recorded.CreatedDate,
		SourceStream:   recorded.StreamID,
		SourceRevision: recorded.EventNumber,
		AccountIDs:     uniqueStrings(accountID),
		Summary:        recorded.EventType,
	}, true
}

func buildGenericLoanItem(recorded *esdb.RecordedEvent) (eventExplorerItemDTO, bool) {
	var raw map[string]any
	if json.Unmarshal(recorded.Data, &raw) != nil {
		raw = map[string]any{}
	}
	occurredAt := recorded.CreatedDate
	if value, ok := raw["occurredAt"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			occurredAt = parsed
		}
	}
	accountID, _ := raw["accountId"].(string)
	loanID, _ := raw["loanId"].(string)
	return eventExplorerItemDTO{
		EventID:        recorded.EventID.String(),
		EventType:      recorded.EventType,
		Category:       "loan",
		OccurredAt:     occurredAt,
		SourceStream:   recorded.StreamID,
		SourceRevision: recorded.EventNumber,
		AccountIDs:     uniqueStrings(accountID),
		LoanID:         loanID,
		Summary:        recorded.EventType,
	}, true
}

func eventExplorerSupportedStream(stream string) bool {
	return stream == journal.BankStream || stream == eventExplorerAuditGlobal || strings.HasPrefix(stream, eventExplorerAccountPrefix) || strings.HasPrefix(stream, auditAccountPrefix) || strings.HasPrefix(stream, "loan-application-")
}

func auditEventCategory(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "Login"):
		return "login"
	case strings.Contains(eventType, "OTP"):
		return "otp"
	case strings.HasPrefix(eventType, "Transfer"):
		return "transfer"
	default:
		return "other"
	}
}

func auditEventDetails(event audit.Event) map[string]any {
	details := map[string]any{}
	if event.OperationID != "" {
		details["operationId"] = event.OperationID
	}
	if event.CorrelationID != "" {
		details["correlationId"] = event.CorrelationID
	}
	if event.ReasonCode != "" {
		details["reasonCode"] = event.ReasonCode
	}
	if event.AttemptsLeft != nil {
		details["attemptsLeft"] = *event.AttemptsLeft
	}
	return details
}

func eventExplorerMatches(item eventExplorerItemDTO, query eventExplorerQuery) bool {
	if query.EventID != "" && !strings.EqualFold(item.EventID, query.EventID) {
		return false
	}
	if query.TransactionID != "" && !strings.EqualFold(item.TransactionID, query.TransactionID) {
		return false
	}
	if query.AccountID != "" && !containsString(item.AccountIDs, query.AccountID) && !containsString(item.AccountIDs, journal.FinancialAccountID(query.AccountID)) {
		return false
	}
	if query.Stream != "" && !strings.EqualFold(item.SourceStream, query.Stream) {
		return false
	}
	if query.Category != "all" && item.Category != query.Category {
		return false
	}
	if query.EventType != "" && !strings.EqualFold(item.EventType, query.EventType) {
		return false
	}
	if !query.From.IsZero() && item.OccurredAt.Before(query.From) {
		return false
	}
	if !query.To.IsZero() && !item.OccurredAt.Before(query.To) {
		return false
	}
	if query.Status != "" {
		status, _ := item.Details["status"].(string)
		if !strings.EqualFold(status, query.Status) {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func (h *TimelineHandler) loadEventExplorerAccounts(ctx context.Context) ([]*accountdomain.UserAccount, error) {
	if h.accounts == nil {
		return []*accountdomain.UserAccount{}, nil
	}
	h.explorerAccountsMu.RLock()
	if !h.explorerAccountsAt.IsZero() && time.Since(h.explorerAccountsAt) < 30*time.Second {
		accounts := append([]*accountdomain.UserAccount(nil), h.explorerAccounts...)
		h.explorerAccountsMu.RUnlock()
		return accounts, nil
	}
	h.explorerAccountsMu.RUnlock()
	accounts, err := h.accounts.FindAll(ctx)
	if err == nil {
		h.explorerAccountsMu.Lock()
		h.explorerAccounts = append([]*accountdomain.UserAccount(nil), accounts...)
		h.explorerAccountsAt = time.Now()
		h.explorerAccountsMu.Unlock()
	}
	return accounts, err
}

func findEventExplorerAccount(accounts []*accountdomain.UserAccount, query string) *accountdomain.UserAccount {
	query = strings.TrimSpace(query)
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if strings.EqualFold(query, account.Id) || strings.EqualFold(query, account.AccountNo) || strings.EqualFold(query, account.Username) || strings.EqualFold(query, account.Email) {
			return account
		}
	}
	return nil
}

func eventExplorerItemsHaveAccounts(items []eventExplorerItemDTO) bool {
	for _, item := range items {
		if len(item.AccountIDs) > 0 {
			return true
		}
	}
	return false
}

func eventExplorerListItems(items []eventExplorerItemDTO) []eventExplorerListItemDTO {
	result := make([]eventExplorerListItemDTO, 0, len(items))
	for _, item := range items {
		result = append(result, eventExplorerListItemDTO{
			EventID:       item.EventID,
			Category:      item.Category,
			OccurredAt:    item.OccurredAt,
			Accounts:      item.Accounts,
			TransactionID: item.TransactionID,
			LoanID:        item.LoanID,
			Summary:       eventExplorerBusinessSummary(item),
		})
	}
	return result
}

func eventExplorerBusinessSummary(item eventExplorerItemDTO) string {
	labels := map[string]string{
		"TransferCreated":                  "Yêu cầu chuyển tiền được tạo",
		"TransferCompleted":                "Chuyển tiền thành công",
		"TransferFailed":                   "Chuyển tiền thất bại",
		"TransferCancelled":                "Chuyển tiền bị hủy",
		"TransferOTPUpdated":               "OTP chuyển tiền được cập nhật",
		"TransferOTPVerificationFailed":    "Xác thực OTP chuyển tiền thất bại",
		"TransferOTPVerificationSucceeded": "Xác thực OTP chuyển tiền thành công",
		"TransferOTPExpired":               "OTP chuyển tiền hết hạn",
		"LoginSucceeded":                   "Đăng nhập thành công",
		"LoginFailed":                      "Đăng nhập thất bại",
	}
	if label := labels[item.EventType]; label != "" {
		return label
	}
	return item.Summary
}

func (h *TimelineHandler) enrichEventExplorerAccounts(items []eventExplorerItemDTO, accounts []*accountdomain.UserAccount) {
	byID := make(map[string]*accountdomain.UserAccount, len(accounts))
	for _, account := range accounts {
		if account != nil {
			byID[account.Id] = account
			byID[journal.FinancialAccountID(account.Id)] = account
		}
	}
	for index := range items {
		refs := make([]eventExplorerAccountDTO, 0, len(items[index].AccountIDs))
		for _, accountID := range items[index].AccountIDs {
			account := byID[accountID]
			if account == nil {
				continue
			}
			name := ""
			if account.Customer != nil {
				name = account.Customer.Profile.FullName
			}
			refs = append(refs, eventExplorerAccountDTO{AccountNo: account.AccountNo, CustomerName: name, Username: account.Username, Role: items[index].AccountRoles[accountID]})
		}
		items[index].Accounts = refs
	}
}

func uniqueStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
