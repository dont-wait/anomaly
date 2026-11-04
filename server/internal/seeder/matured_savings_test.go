package seeder

import (
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/dont-wait/anomaly/internal/timeutil"
)

func TestMaturedTodayOpeningMaturesEarlierToday(t *testing.T) {
	vn := timeutil.VietnamLocation
	for _, now := range []time.Time{
		time.Date(2026, 11, 3, 21, 30, 0, 0, vn),
		time.Date(2026, 11, 3, 0, 20, 0, 0, vn),
		time.Date(2026, 3, 1, 9, 0, 0, 0, vn),
		time.Date(2027, 1, 31, 15, 0, 0, 0, vn),
	} {
		openedAt, err := maturedTodayOpening(now, 3)
		if err != nil {
			t.Fatalf("%s: %v", now, err)
		}
		maturity, err := savings.MaturityForStoredContract(openedAt, 3)
		if err != nil {
			t.Fatal(err)
		}
		m := maturity.In(vn)
		if m.After(now) || m.YearDay() != now.YearDay() || m.Year() != now.Year() {
			t.Fatalf("now %s: maturity %s is not earlier today", now, m)
		}
	}
}
