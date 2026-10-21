package loan

import (
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
)

func TestCalculateDueDateVietnamBusinessDay(t *testing.T) {
	vn := timeutil.VietnamLocation

	tests := []struct {
		name  string
		at    time.Time
		month int
		want  string
	}{
		{
			"UTC previous calendar day",
			time.Date(2026, 10, 14, 18, 0, 0, 0, time.UTC),
			1, "2026-11-15",
		},
		{
			"Vietnam 00:30",
			time.Date(2026, 10, 15, 0, 30, 0, 0, vn),
			1, "2026-11-15",
		},
		{
			"Jan 31 to Feb non-leap",
			time.Date(2027, 1, 31, 10, 0, 0, 0, vn),
			1, "2027-02-28",
		},
		{
			"Jan 31 to Feb leap",
			time.Date(2028, 1, 31, 10, 0, 0, 0, vn),
			1, "2028-02-29",
		},
		{
			"Mar 30 to April",
			time.Date(2027, 3, 30, 10, 0, 0, 0, vn),
			1, "2027-04-30",
		},
		{
			"Jan 29 non leap",
			time.Date(2027, 1, 29, 10, 0, 0, 0, vn),
			1, "2027-02-28",
		},
		{
			"Jan 28 non leap",
			time.Date(2027, 1, 28, 10, 0, 0, 0, vn),
			1, "2027-02-28",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateDueDate(
				tc.at, tc.month,
			).UTC().Format("2006-01-02")

			if got != tc.want {
				t.Fatalf(
					"due = %s, want %s",
					got, tc.want,
				)
			}
		})
	}
}

func TestBusinessDayStableAcrossTimeZones(t *testing.T) {
	vn := timeutil.VietnamLocation

	disbursed := time.Date(
		2026, 10, 15, 0, 30, 0, 0, vn,
	)

	for _, at := range []time.Time{
		disbursed,
		disbursed.UTC(),
	} {
		schedule, err := CalculateRepaymentSchedule(
			60_000_000,
			1630,
			12,
			at,
		)
		if err != nil {
			t.Fatal(err)
		}

		got := schedule[0].DueDate.UTC().
			Format("2006-01-02")

		if got != "2026-11-15" {
			t.Fatalf("first due date = %s", got)
		}
	}
}
