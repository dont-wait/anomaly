package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/reporting"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

type reportReaderFake struct {
	calls    int
	from, to time.Time
	err      error
}

func (f *reportReaderFake) TransactionReport(ctx context.Context, from, to time.Time) (reporting.Report, error) {
	f.calls++
	f.from, f.to = from, to
	if _, ok := ctx.Deadline(); !ok {
		return reporting.Report{}, errors.New("report read must have a deadline")
	}
	return reporting.NewBuilder(from, to, from.Add(time.Hour), nil).Finish(from.Add(2 * time.Hour)), f.err
}

func TestReportValidatesDateBeforeQuery(t *testing.T) {
	for _, query := range []string{
		"", "date=", "date=2026-02-30", "date=2026-1-08",
		"date=2026-11-08T00:00:00Z", "date=%202026-11-08",
	} {
		t.Run(query, func(t *testing.T) {
			reader := &reportReaderFake{}
			handler := &MonitorHandler{reports: reader}
			response := httptest.NewRecorder()
			handler.TransactionReport(response, httptest.NewRequest(http.MethodGet, "/api/admin/reports/transactions?"+query, nil))
			if response.Code != http.StatusBadRequest || reader.calls != 0 {
				t.Fatalf("status %d, queries %d: %s", response.Code, reader.calls, response.Body.String())
			}
		})
	}
}

func TestReportEnvelopeWindowAndEmptyData(t *testing.T) {
	reader := &reportReaderFake{}
	handler := &MonitorHandler{reports: reader}
	response := httptest.NewRecorder()
	handler.TransactionReport(response, httptest.NewRequest(http.MethodGet, "/api/admin/reports/transactions?date=2026-11-08", nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status/headers: %d/%v: %s", response.Code, response.Header(), response.Body.String())
	}
	wantFrom := time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC)
	if reader.calls != 1 || !reader.from.Equal(wantFrom) || !reader.to.Equal(wantFrom.Add(24*time.Hour)) {
		t.Fatalf("Vietnam day: %s to %s, calls=%d", reader.from, reader.to, reader.calls)
	}
	var envelope httpx.SuccessResponse[reporting.Report]
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != 200 || envelope.Data.ReportVersion != 2 || envelope.Data.Date != "2026-11-08" ||
		envelope.Data.PreviousDate != "2026-11-07" || envelope.Data.Timezone != "Asia/Ho_Chi_Minh" ||
		len(envelope.Data.Hourly.Created) != 24 || envelope.Data.Summary.Risk.AverageScore != nil ||
		envelope.Data.Projection.Status != "unknown" || envelope.Data.Projection.CheckpointRevision != nil {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestReportReadFailureIsNotEmptySuccess(t *testing.T) {
	handler := &MonitorHandler{reports: &reportReaderFake{err: errors.New("database unavailable")}}
	response := httptest.NewRecorder()
	handler.TransactionReport(response, httptest.NewRequest(http.MethodGet, "/api/admin/reports/transactions?date=2026-11-08", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

func TestReportHasOnlyOneContract(t *testing.T) {
	// An obsolete version query cannot select the removed implementation.
	for _, version := range []string{"", "&version=1", "&version=2"} {
		reader := &reportReaderFake{}
		handler := &MonitorHandler{reports: reader}
		response := httptest.NewRecorder()
		handler.TransactionReport(response, httptest.NewRequest(http.MethodGet, "/api/admin/reports/transactions?date=2026-11-08"+version, nil))
		var envelope httpx.SuccessResponse[reporting.Report]
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || reader.calls != 1 || envelope.Data.ReportVersion != 2 {
			t.Fatalf("unexpected contract for %q: %s", version, response.Body.String())
		}
	}
}
