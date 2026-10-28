package admin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/application/riskstream"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

type MonitorHandler struct {
	repository *mongorepo.RiskAssessmentRepository
	hub        *riskstream.Hub
}

func NewMonitorHandler(repository *mongorepo.RiskAssessmentRepository, hub *riskstream.Hub) *MonitorHandler {
	return &MonitorHandler{repository: repository, hub: hub}
}

type cursor struct {
	ReceivedAt time.Time `json:"receivedAt"`
	ID         string    `json:"id"`
}

type riskAssessmentDTO struct {
	AssessmentID   string    `json:"assessmentId"`
	TransactionID  string    `json:"transactionId"`
	RiskScore      float64   `json:"riskScore"`
	PredictedLabel int       `json:"predictedLabel"`
	Threshold      float64   `json:"threshold"`
	ModelVersion   string    `json:"modelVersion"`
	FeatureVersion string    `json:"featureVersion"`
	ReceivedAt     time.Time `json:"receivedAt"`
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
}

type monitorResponse struct {
	Range      string          `json:"range"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	DataStatus string          `json:"dataStatus"`
	Cursor     string          `json:"cursor"`
	Snapshot   monitorSnapshot `json:"snapshot"`
}

func RegisterRoutes(router *openapi.Registry, handler *MonitorHandler, tokenSvc queries.TokenService, currentAdmin func(http.Handler) http.Handler) {
	admin := func(pattern string, fn http.HandlerFunc, operation openapi.Operation) {
		operation.Auth = true
		operation.Tags = []string{"Admin Monitor"}
		secured := middleware.RequireAuth(tokenSvc)(currentAdmin(http.HandlerFunc(fn)))
		router.Handle(pattern, secured, operation)
	}
	admin("GET /api/admin/monitor", handler.Snapshot, openapi.Operation{
		ID: "adminMonitorSnapshot", Summary: "Get risk assessment monitor snapshot",
		Parameters: []openapi.Parameter{{Name: "range", In: "query", Type: "string", Example: "24h"}},
		Response:   monitorResponse{}, FailureStatuses: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
	})
	admin("GET /api/admin/monitor/stream", handler.Stream, openapi.Operation{
		ID: "adminMonitorStream", Summary: "Stream risk assessments to the admin monitor",
		ResponseContentType: "text/event-stream", FailureStatuses: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
	})
}

func (h *MonitorHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	duration, rangeName, ok := monitorRange(r.URL.Query().Get("range"))
	if !ok {
		http.Error(w, "range must be 24h, 7d, or 30d", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	from := now.Add(-duration)
	summary, err := h.repository.Summarize(r.Context(), from, now)
	if err != nil {
		http.Error(w, "could not load risk assessments", http.StatusInternalServerError)
		return
	}
	recent, err := h.repository.ListRecent(r.Context(), from, now, 50)
	if err != nil {
		http.Error(w, "could not load recent risk assessments", http.StatusInternalServerError)
		return
	}
	response := buildMonitorResponse(rangeName, summary, recent, now)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
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

	updates, cancel := h.hub.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	if after.ID != "" || !after.ReceivedAt.IsZero() {
		for {
			items, listErr := h.repository.ListAfter(r.Context(), after.ReceivedAt, after.ID, 1000)
			if listErr != nil {
				return
			}
			for _, item := range items {
				if err := writeAssessmentEvent(w, flusher, item); err != nil {
					return
				}
				after = cursor{ReceivedAt: item.ReceivedAt, ID: item.ID}
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
			if !isAfter(event.Assessment, after) {
				continue
			}
			if err := writeAssessmentEvent(w, flusher, event.Assessment); err != nil {
				return
			}
			after = cursor{ReceivedAt: event.Assessment.ReceivedAt, ID: event.Assessment.ID}
		case <-ticker.C:
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

func buildMonitorResponse(rangeName string, summary mongorepo.RiskAssessmentSummary, recent []riskassessment.Assessment, generatedAt time.Time) monitorResponse {
	snapshot := monitorSnapshot{
		TotalAssessments:  int(summary.Total),
		AnomalousCount:    int(summary.Anomalous),
		RecentAssessments: make([]riskAssessmentDTO, 0, len(recent)),
		Timeseries:        make([]monitorPoint, 0, len(summary.Timeseries)),
		ModelVersions:     make([]modelVersionCount, 0, len(summary.ModelVersions)),
	}
	if summary.Total > 0 {
		snapshot.AverageRiskScore = summary.ScoreTotal / float64(summary.Total)
		snapshot.AnomalyRate = float64(summary.Anomalous) / float64(summary.Total)
	}
	for _, item := range recent {
		snapshot.RecentAssessments = append(snapshot.RecentAssessments, toDTO(item))
	}
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
	if len(recent) > 0 {
		latest := recent[len(recent)-1]
		return monitorResponse{Range: rangeName, UpdatedAt: latest.ReceivedAt, DataStatus: "fresh", Cursor: encodeCursor(cursor{ReceivedAt: latest.ReceivedAt, ID: latest.ID}), Snapshot: snapshot}
	}
	watermark := generatedAt.UTC().Truncate(time.Millisecond)
	return monitorResponse{Range: rangeName, UpdatedAt: watermark, DataStatus: "fresh", Cursor: encodeCursor(cursor{ReceivedAt: watermark}), Snapshot: snapshot}
}

func toDTO(item riskassessment.Assessment) riskAssessmentDTO {
	return riskAssessmentDTO{AssessmentID: item.ID, TransactionID: item.TransactionID, RiskScore: item.RiskScore, PredictedLabel: item.PredictedLabel, Threshold: item.Threshold, ModelVersion: item.ModelVersion, FeatureVersion: item.FeatureVersion, ReceivedAt: item.ReceivedAt}
}

func writeAssessmentEvent(w http.ResponseWriter, flusher http.Flusher, item riskassessment.Assessment) error {
	payload, err := json.Marshal(toDTO(item))
	if err != nil {
		return err
	}
	id := encodeCursor(cursor{ReceivedAt: item.ReceivedAt, ID: item.ID})
	if _, err := fmt.Fprintf(w, "id: %s\nevent: risk_assessment.created\ndata: %s\n\n", id, payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func encodeCursor(value cursor) string {
	if value.ReceivedAt.IsZero() {
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
	if result.ReceivedAt.IsZero() {
		return cursor{}, fmt.Errorf("cursor fields are required")
	}
	return result, nil
}

func isAfter(item riskassessment.Assessment, after cursor) bool {
	if after.ReceivedAt.IsZero() {
		return true
	}
	if item.ReceivedAt.After(after.ReceivedAt) {
		return true
	}
	return item.ReceivedAt.Equal(after.ReceivedAt) && (after.ID == "" || item.ID > after.ID)
}
