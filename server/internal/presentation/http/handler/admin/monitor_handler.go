package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/application/riskstream"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	"github.com/dont-wait/anomaly/internal/timeutil"
	"github.com/rs/zerolog"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

type MonitorHandler struct {
	repository   *mongorepo.RiskAssessmentRepository
	hub          *riskstream.Hub
	transactions transactionContextLookup
	currentAdmin func(context.Context, string) bool
	audit        audit.Recorder
}

func (h *MonitorHandler) WithAudit(recorder audit.Recorder) *MonitorHandler {
	h.audit = recorder
	return h
}

type transactionContextLookup interface {
	FindRiskTransactionContexts(context.Context, []string) (map[string]mongorepo.RiskTransactionContext, error)
}

func NewMonitorHandler(repository *mongorepo.RiskAssessmentRepository, hub *riskstream.Hub, transactions transactionContextLookup, currentAdmin ...func(context.Context, string) bool) *MonitorHandler {
	var checker func(context.Context, string) bool
	if len(currentAdmin) > 0 {
		checker = currentAdmin[0]
	}
	return &MonitorHandler{repository: repository, hub: hub, transactions: transactions, currentAdmin: checker}
}

type cursor struct {
	Sequence   int64     `json:"sequence,omitempty"`
	ReceivedAt time.Time `json:"receivedAt"`
	ID         string    `json:"id"`
}

type riskAssessmentDTO struct {
	AssessmentID   string                 `json:"assessmentId"`
	TransactionID  string                 `json:"transactionId"`
	RiskScore      float64                `json:"riskScore"`
	PredictedLabel int                    `json:"predictedLabel"`
	RiskLevel      string                 `json:"riskLevel"`
	Decision       string                 `json:"decision"`
	Threshold      float64                `json:"threshold"`
	ModelVersion   string                 `json:"modelVersion"`
	FeatureVersion string                 `json:"featureVersion"`
	ReceivedAt     time.Time              `json:"receivedAt"`
	Handled        bool                   `json:"handled"`
	HandledAt      *time.Time             `json:"handledAt,omitempty"`
	HandledBy      string                 `json:"handledBy,omitempty"`
	Transaction    *transactionContextDTO `json:"transaction"`
	Cursor         int64                  `json:"-"`
}

type transactionContextDTO struct {
	TransactionID            string     `json:"transactionId"`
	SourceAccountID          string     `json:"sourceAccountId"`
	SourceAccountStatus      string     `json:"sourceAccountStatus"`
	SourceAccountNo          string     `json:"sourceAccountNo"`
	SourceAccountName        string     `json:"sourceAccountName"`
	DestinationAccountID     string     `json:"destinationAccountId"`
	DestinationAccountStatus string     `json:"destinationAccountStatus"`
	DestinationAccountNo     string     `json:"destinationAccountNo"`
	DestinationName          string     `json:"destinationName"`
	Amount                   int64      `json:"amount"`
	Fee                      int64      `json:"fee"`
	Currency                 string     `json:"currency"`
	Channel                  string     `json:"channel"`
	Status                   string     `json:"status"`
	CreatedAt                time.Time  `json:"createdAt"`
	OTPExpiresAt             *time.Time `json:"otpExpiresAt,omitempty"`
	OTPVerifiedAt            *time.Time `json:"otpVerifiedAt,omitempty"`
	CompletedAt              *time.Time `json:"completedAt,omitempty"`
	FailedAt                 *time.Time `json:"failedAt,omitempty"`
	CancelledAt              *time.Time `json:"cancelledAt,omitempty"`
}

type monitorPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	AssessmentCount  int       `json:"assessmentCount"`
	AnomalousCount   int       `json:"anomalousCount"`
	AverageRiskScore float64   `json:"averageRiskScore"`
}

type modelVersionCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type monitorSnapshot struct {
	TotalAssessments  int                 `json:"totalAssessments"`
	AnomalousCount    int                 `json:"anomalousCount"`
	AnomalyRate       float64             `json:"anomalyRate"`
	AverageRiskScore  float64             `json:"averageRiskScore"`
	Timeseries        []monitorPoint      `json:"timeseries"`
	RecentAssessments []riskAssessmentDTO `json:"recentAssessments"`
	ModelVersions     []modelVersionCount `json:"modelVersions"`
	NormalCount       int                 `json:"normalCount"`
	WarningCount      int                 `json:"warningCount"`
	BlockCount        int                 `json:"blockCount"`
	TransactionStatus map[string]int      `json:"transactionStatus"`
}

type monitorResponse struct {
	Range      string          `json:"range"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	DataStatus string          `json:"dataStatus"`
	Cursor     string          `json:"cursor"`
	Snapshot   monitorSnapshot `json:"snapshot"`
}

type transactionReportSummaryDTO struct {
	TransactionCount int64   `json:"transactionCount"`
	TotalAmount      int64   `json:"totalAmount"`
	TotalFee         int64   `json:"totalFee"`
	AverageAmount    float64 `json:"averageAmount"`
	SuccessCount     int64   `json:"successCount"`
	FailedCount      int64   `json:"failedCount"`
	BlockedCount     int64   `json:"blockedCount"`
}

type transactionReportDTO struct {
	Date             string                              `json:"date"`
	PreviousDate     string                              `json:"previousDate"`
	Summary          transactionReportSummaryDTO         `json:"summary"`
	Previous         transactionReportSummaryDTO         `json:"previous"`
	Hourly           []mongorepo.TransactionReportHourly `json:"hourly"`
	Transactions     []mongorepo.TransactionReportItem   `json:"transactions"`
	TopRiskSources   []mongorepo.TransactionRiskAccount  `json:"topRiskSources"`
	TopRiskReceivers []mongorepo.TransactionRiskAccount  `json:"topRiskReceivers"`
}

func RegisterRoutes(router *openapi.Registry, handler *MonitorHandler, tokenSvc queries.TokenService, currentAdmin func(http.Handler) http.Handler, timelines ...*TimelineHandler) {
	admin := func(pattern string, fn http.HandlerFunc, operation openapi.Operation) {
		operation.Auth = true
		operation.Tags = []string{"Admin Monitor"}
		secured := middleware.RequireAuth(tokenSvc)(currentAdmin(http.HandlerFunc(fn)))
		router.Handle(pattern, secured, operation)
	}
	admin("GET /api/admin/monitor", handler.Snapshot, openapi.Operation{
		ID: "adminMonitorSnapshot", Summary: "Get risk assessment monitor snapshot",
		Parameters: []openapi.Parameter{
			{Name: "range", In: "query", Type: "string", Example: "24h"},
			{Name: "date", In: "query", Type: "string", Format: "date", Example: "2026-11-04"},
		},
		Response: monitorResponse{}, FailureStatuses: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
	})
	admin("GET /api/admin/reports/transactions", handler.TransactionReport, openapi.Operation{
		ID: "adminTransactionReport", Summary: "Get daily transaction and risk report",
		Parameters: []openapi.Parameter{{Name: "date", In: "query", Type: "string", Format: "date", Required: true, Example: "2026-11-04"}},
		Response:   transactionReportDTO{}, FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
	})
	admin("GET /api/admin/risk-assessments/transactions/{transactionId}", handler.TransactionAssessment, openapi.Operation{
		ID: "adminTransactionRiskAssessment", Summary: "Get latest risk assessment for a transaction",
		Parameters: []openapi.Parameter{{Name: "transactionId", In: "path", Type: "string", Required: true}},
		Response:   riskAssessmentDTO{}, FailureStatuses: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	})
	admin("PATCH /api/admin/risk-assessments/{id}/handled", handler.UpdateHandled, openapi.Operation{
		ID: "updateRiskAssessmentHandled", Summary: "Mark a risk assessment handled or reopen it",
		Request: struct {
			Handled bool `json:"handled"`
		}{}, Response: map[string]any{},
		FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	})
	admin("GET /api/admin/monitor/stream", handler.Stream, openapi.Operation{
		ID: "adminMonitorStream", Summary: "Stream risk assessments to the admin monitor",
		ResponseContentType: "text/event-stream", FailureStatuses: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
	})
	if len(timelines) > 0 && timelines[0] != nil {
		RegisterTimelineRoutes(router, timelines[0], tokenSvc, currentAdmin)
		RegisterEventExplorerRoutes(router, timelines[0], tokenSvc, currentAdmin)
	}
}

func (h *MonitorHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	queryCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	from, to, rangeName, err := monitorWindow(r.URL.Query().Get("date"), r.URL.Query().Get("range"), time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	summary, err := h.repository.Summarize(queryCtx, from, to)
	if err != nil {
		http.Error(w, "could not load risk assessments", http.StatusInternalServerError)
		return
	}
	var handled *bool
	switch r.URL.Query().Get("handled") {
	case "", "all":
	case "true", "handled":
		value := true
		handled = &value
	case "false", "unhandled":
		value := false
		handled = &value
	default:
		http.Error(w, "handled must be all, handled, or unhandled", http.StatusBadRequest)
		return
	}
	recent, err := h.repository.ListRecentByHandled(queryCtx, from, to, 50, handled)
	if err != nil {
		http.Error(w, "could not load recent risk assessments", http.StatusInternalServerError)
		return
	}
	recentDTO, err := h.enrichAssessments(queryCtx, recent)
	if err != nil {
		http.Error(w, "could not load transaction context", http.StatusInternalServerError)
		return
	}
	response := buildMonitorResponse(rangeName, summary, recentDTO, time.Now().UTC())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *MonitorHandler) TransactionReport(w http.ResponseWriter, r *http.Request) {
	queryCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	from, to, date, err := monitorWindow(r.URL.Query().Get("date"), "", time.Now().UTC())
	if err != nil {
		httpx.WriteError(w, zerolog.Nop(), err, func(error) int { return http.StatusBadRequest })
		return
	}
	previousFrom := from.AddDate(0, 0, -1)
	report, err := h.repository.TransactionReport(queryCtx, from, to, previousFrom, from)
	if err != nil {
		httpx.WriteError(w, zerolog.Nop(), err, func(error) int { return http.StatusInternalServerError })
		return
	}
	toSummary := func(value mongorepo.TransactionReportSummary) transactionReportSummaryDTO {
		return transactionReportSummaryDTO{
			TransactionCount: value.TransactionCount,
			TotalAmount:      value.TotalAmount,
			TotalFee:         value.TotalFee,
			AverageAmount:    value.AverageAmount,
			SuccessCount:     value.SuccessCount,
			FailedCount:      value.FailedCount,
			BlockedCount:     value.BlockedCount,
		}
	}
	response := transactionReportDTO{
		Date: date, PreviousDate: previousFrom.In(timeutil.VietnamLocation).Format("2006-01-02"),
		Summary: toSummary(report.Summary), Previous: toSummary(report.Previous),
		Hourly:         report.Hourly,
		Transactions:   report.Transactions,
		TopRiskSources: report.TopRiskSources, TopRiskReceivers: report.TopRiskReceivers,
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, http.StatusOK, "Transaction report loaded", response)
}

func (h *MonitorHandler) TransactionAssessment(w http.ResponseWriter, r *http.Request) {
	transactionID := strings.TrimSpace(r.PathValue("transactionId"))
	if transactionID == "" {
		httpx.WriteError(w, zerolog.Nop(), errors.New("transaction id is required"), func(error) int { return http.StatusBadRequest })
		return
	}
	assessment, found, err := h.repository.FindLatestAssessment(r.Context(), transactionID)
	if err != nil {
		httpx.WriteError(w, zerolog.Nop(), err, func(error) int { return http.StatusInternalServerError })
		return
	}
	if !found {
		httpx.WriteError(w, zerolog.Nop(), mongodrv.ErrNoDocuments, func(error) int { return http.StatusNotFound })
		return
	}
	items, err := h.enrichAssessments(r.Context(), []riskassessment.Assessment{assessment})
	if err != nil || len(items) == 0 {
		if err == nil {
			err = errors.New("risk assessment context not found")
		}
		httpx.WriteError(w, zerolog.Nop(), err, func(error) int { return http.StatusInternalServerError })
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, "Risk assessment loaded", items[0])
}

func (h *MonitorHandler) UpdateHandled(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var request struct {
		Handled bool `json:"handled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid handled payload", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	if err := h.repository.SetHandled(r.Context(), id, request.Handled, claims.UserID); err != nil {
		if errors.Is(err, mongodrv.ErrNoDocuments) {
			http.Error(w, "risk assessment not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not update risk assessment", http.StatusInternalServerError)
		return
	}
	if h.audit != nil {
		eventType := audit.RiskAssessmentReopened
		if request.Handled {
			eventType = audit.RiskAssessmentHandled
		}
		event := audit.NewEvent(eventType, "", time.Now().UTC())
		event.ActorID = claims.UserID
		event.ReasonCode = id
		_ = h.audit.Record(r.Context(), event)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"assessmentId": id, "handled": request.Handled})
}

func (h *MonitorHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	rawCursor := r.URL.Query().Get("cursor")
	if rawCursor == "" {
		rawCursor = r.Header.Get("Last-Event-ID")
	}
	after, err := decodeCursor(rawCursor)
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}
	claims, claimsOK := middleware.ClaimsFromContext(r.Context())
	if !claimsOK || claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	updates, cancel := h.hub.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	setSSEWriteDeadline(w)
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	if after.Sequence > 0 || after.ID != "" || !after.ReceivedAt.IsZero() {
		for {
			queryCtx, queryCancel := context.WithTimeout(r.Context(), 5*time.Second)
			items, listErr := h.repository.ListAfter(queryCtx, after.Sequence, after.ReceivedAt, after.ID, 1000)
			if listErr != nil {
				queryCancel()
				return
			}
			dtos, enrichErr := h.enrichAssessments(queryCtx, items)
			queryCancel()
			if enrichErr != nil {
				return
			}
			for index, item := range items {
				if err := writeAssessmentEvent(w, flusher, item, dtos[index]); err != nil {
					return
				}
				after = cursor{Sequence: item.Cursor, ReceivedAt: item.ReceivedAt, ID: item.ID}
			}
			if len(items) < 1000 {
				break
			}
		}
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-updates:
			if !open {
				return
			}
			if event.TransactionStatus != nil {
				if err := writeTransactionStatusEvent(w, flusher, *event.TransactionStatus); err != nil {
					return
				}
				continue
			}
			if !isAfter(event.Assessment, after) {
				continue
			}
			queryCtx, queryCancel := context.WithTimeout(r.Context(), 5*time.Second)
			dto, enrichErr := h.enrichAssessment(queryCtx, event.Assessment)
			queryCancel()
			if enrichErr != nil {
				return
			}
			if err := writeAssessmentEvent(w, flusher, event.Assessment, dto); err != nil {
				return
			}
			after = cursor{Sequence: event.Assessment.Cursor, ReceivedAt: event.Assessment.ReceivedAt, ID: event.Assessment.ID}
		case <-ticker.C:
			if !claims.ExpiresAt.IsZero() && !time.Now().Before(claims.ExpiresAt) {
				return
			}
			if h.currentAdmin != nil && !h.currentAdmin(r.Context(), claims.UserID) {
				return
			}
			setSSEWriteDeadline(w)
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func monitorRange(value string) (time.Duration, string, bool) {
	switch value {
	case "", "24h":
		return 24 * time.Hour, "24h", true
	case "7d":
		return 7 * 24 * time.Hour, "7d", true
	case "30d":
		return 30 * 24 * time.Hour, "30d", true
	default:
		return 0, "", false
	}
}

func monitorWindow(dateValue, rangeValue string, now time.Time) (time.Time, time.Time, string, error) {
	if strings.TrimSpace(dateValue) != "" {
		date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(dateValue), timeutil.VietnamLocation)
		if err != nil {
			return time.Time{}, time.Time{}, "", errors.New("date must use YYYY-MM-DD")
		}
		next := date.AddDate(0, 0, 1)
		return date.UTC(), next.UTC(), date.Format("2006-01-02"), nil
	}
	duration, rangeName, ok := monitorRange(rangeValue)
	if !ok {
		return time.Time{}, time.Time{}, "", errors.New("range must be 24h, 7d, or 30d")
	}
	to := now.UTC()
	return to.Add(-duration), to, rangeName, nil
}

func buildMonitorResponse(rangeName string, summary mongorepo.RiskAssessmentSummary, recent []riskAssessmentDTO, generatedAt time.Time) monitorResponse {
	snapshot := monitorSnapshot{
		TotalAssessments:  int(summary.Total),
		AnomalousCount:    int(summary.Anomalous),
		RecentAssessments: make([]riskAssessmentDTO, 0, len(recent)),
		Timeseries:        make([]monitorPoint, 0, len(summary.Timeseries)),
		ModelVersions:     make([]modelVersionCount, 0, len(summary.ModelVersions)),
		TransactionStatus: make(map[string]int, len(summary.Statuses)),
	}
	if summary.Total > 0 {
		snapshot.AverageRiskScore = summary.ScoreTotal / float64(summary.Total)
		snapshot.AnomalyRate = float64(summary.Anomalous) / float64(summary.Total)
	}
	snapshot.RecentAssessments = append(snapshot.RecentAssessments, recent...)
	for _, item := range summary.Timeseries {
		average := 0.0
		if item.AssessmentCount > 0 {
			average = item.ScoreTotal / float64(item.AssessmentCount)
		}
		snapshot.Timeseries = append(snapshot.Timeseries, monitorPoint{
			Timestamp: item.Timestamp, AssessmentCount: item.AssessmentCount,
			AnomalousCount: item.AnomalousCount, AverageRiskScore: average,
		})
	}
	for _, item := range summary.ModelVersions {
		snapshot.ModelVersions = append(snapshot.ModelVersions, modelVersionCount{Label: item.Label, Count: item.Count})
	}
	for _, item := range summary.RiskLevels {
		switch item.Label {
		case riskassessment.RiskLevelNormal:
			snapshot.NormalCount = item.Count
		case riskassessment.RiskLevelWarning:
			snapshot.WarningCount = item.Count
		case riskassessment.RiskLevelBlock:
			snapshot.BlockCount = item.Count
		}
	}
	for _, item := range summary.Statuses {
		if item.Label != "" {
			snapshot.TransactionStatus[item.Label] = item.Count
		}
	}
	if len(recent) > 0 {
		latest := recent[len(recent)-1]
		return monitorResponse{Range: rangeName, UpdatedAt: latest.ReceivedAt, DataStatus: "fresh", Cursor: encodeCursor(cursor{Sequence: latest.Cursor, ReceivedAt: latest.ReceivedAt, ID: latest.AssessmentID}), Snapshot: snapshot}
	}
	watermark := generatedAt.UTC().Truncate(time.Millisecond)
	return monitorResponse{Range: rangeName, UpdatedAt: watermark, DataStatus: "fresh", Cursor: encodeCursor(cursor{ReceivedAt: watermark}), Snapshot: snapshot}
}

func (h *MonitorHandler) enrichAssessments(ctx context.Context, items []riskassessment.Assessment) ([]riskAssessmentDTO, error) {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.TransactionID)
	}
	contexts := map[string]mongorepo.RiskTransactionContext{}
	if h.transactions != nil {
		var err error
		contexts, err = h.transactions.FindRiskTransactionContexts(ctx, ids)
		if err != nil {
			return nil, err
		}
	}
	result := make([]riskAssessmentDTO, 0, len(items))
	for _, item := range items {
		result = append(result, h.assessmentDTO(item, contexts[item.TransactionID]))
	}
	return result, nil
}

func (h *MonitorHandler) enrichAssessment(ctx context.Context, item riskassessment.Assessment) (riskAssessmentDTO, error) {
	dtos, err := h.enrichAssessments(ctx, []riskassessment.Assessment{item})
	if err != nil {
		return riskAssessmentDTO{}, err
	}
	return dtos[0], nil
}

func (h *MonitorHandler) assessmentDTO(item riskassessment.Assessment, transaction mongorepo.RiskTransactionContext) riskAssessmentDTO {
	dto := riskAssessmentDTO{AssessmentID: item.ID, TransactionID: item.TransactionID, RiskScore: item.RiskScore, PredictedLabel: item.PredictedLabel, RiskLevel: item.RiskLevel, Decision: item.Decision, Threshold: item.Threshold, ModelVersion: item.ModelVersion, FeatureVersion: item.FeatureVersion, ReceivedAt: item.ReceivedAt, Handled: item.Handled, HandledAt: item.HandledAt, HandledBy: item.HandledBy, Cursor: item.Cursor}
	if transaction.TransactionID != "" {
		dto.Transaction = &transactionContextDTO{TransactionID: transaction.TransactionID, SourceAccountID: transaction.SourceAccountID, SourceAccountNo: transaction.SourceAccountNo, SourceAccountName: transaction.SourceAccountName, DestinationAccountID: transaction.DestinationAccountID, DestinationAccountNo: transaction.DestinationAccountNo, DestinationName: transaction.DestinationName, Amount: transaction.Amount, Fee: transaction.Fee, Currency: transaction.Currency, Channel: transaction.Channel, Status: transaction.Status, CreatedAt: transaction.CreatedAt, OTPExpiresAt: transaction.OTPExpiresAt, OTPVerifiedAt: transaction.OTPVerifiedAt, CompletedAt: transaction.CompletedAt, FailedAt: transaction.FailedAt, CancelledAt: transaction.CancelledAt}
	}
	if dto.Transaction != nil {
		dto.Transaction.SourceAccountStatus = transaction.SourceAccountStatus
		dto.Transaction.DestinationAccountStatus = transaction.DestinationAccountStatus
	}
	return dto
}

func writeAssessmentEvent(w http.ResponseWriter, flusher http.Flusher, item riskassessment.Assessment, dto riskAssessmentDTO) error {
	setSSEWriteDeadline(w)
	payload, err := json.Marshal(dto)
	if err != nil {
		return err
	}
	id := encodeCursor(cursor{Sequence: item.Cursor, ReceivedAt: item.ReceivedAt, ID: item.ID})
	if _, err := fmt.Fprintf(w, "id: %s\nevent: risk_assessment.created\ndata: %s\n\n", id, payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeTransactionStatusEvent(w http.ResponseWriter, flusher http.Flusher, status riskstream.TransactionStatus) error {
	setSSEWriteDeadline(w)
	payload, err := json.Marshal(status)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: transaction.status_changed\ndata: %s\n\n", payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func setSSEWriteDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
}

func encodeCursor(value cursor) string {
	if value.Sequence <= 0 && value.ReceivedAt.IsZero() {
		return ""
	}
	payload, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(value string) (cursor, error) {
	if strings.TrimSpace(value) == "" {
		return cursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor{}, err
	}
	var result cursor
	if err := json.Unmarshal(payload, &result); err != nil {
		return cursor{}, err
	}
	if result.Sequence <= 0 && result.ReceivedAt.IsZero() {
		return cursor{}, fmt.Errorf("cursor fields are required")
	}
	return result, nil
}

func isAfter(item riskassessment.Assessment, after cursor) bool {
	if after.Sequence > 0 {
		return item.Cursor > after.Sequence
	}
	if after.ReceivedAt.IsZero() {
		return true
	}
	if item.ReceivedAt.After(after.ReceivedAt) {
		return true
	}
	return item.ReceivedAt.Equal(after.ReceivedAt) && (after.ID == "" || item.ID > after.ID)
}
