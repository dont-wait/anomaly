package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/timeutil"
	"github.com/rs/zerolog"
)

func reportDay(value string) (time.Time, time.Time, error) {
	date, err := time.ParseInLocation(time.DateOnly, value, timeutil.VietnamLocation)
	if err != nil || date.Format(time.DateOnly) != value {
		return time.Time{}, time.Time{}, errors.New("date is required and must use YYYY-MM-DD")
	}
	return date.UTC(), date.AddDate(0, 0, 1).UTC(), nil
}

func (h *MonitorHandler) TransactionReport(w http.ResponseWriter, r *http.Request) {
	from, to, err := reportDay(r.URL.Query().Get("date"))
	if err != nil {
		httpx.WriteError(w, zerolog.Nop(), err, func(error) int { return http.StatusBadRequest })
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	report, err := h.reports.TransactionReport(ctx, from, to)
	if err != nil {
		httpx.WriteError(w, zerolog.Nop(), err, func(error) int { return http.StatusInternalServerError })
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, http.StatusOK, "Transaction report loaded", report)
}
