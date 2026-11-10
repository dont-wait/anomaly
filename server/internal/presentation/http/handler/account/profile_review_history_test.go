package account

import (
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"testing"
	"time"
)

func TestApprovedHistoryUsesImmutableBeforeImage(t *testing.T) {
	at := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	params := profileChangeListParams{Page: 1, PageSize: 10, Status: "APPROVED"}
	result := profileChangePageFromStore(accountdomain.ProfileChangeListResult{
		Items: []accountdomain.ProfileChangeReview{{
			AccountID: "acc", Username: "anna", HasPrevious: true,
			Previous: accountdomain.ProfileChangeSnapshot{FullName: "Tên trước duyệt"},
			Change:   accountdomain.ProfileChangeRequest{Id: "r1", FullName: "Tên đã duyệt", Status: "APPROVED", RequestedAt: at},
		}}, Total: 1,
	}, params)
	if !result.Items[0].CurrentKnown || result.Items[0].Current.FullName != "Tên trước duyệt" || result.Items[0].Change.FullName != "Tên đã duyệt" {
		t.Fatalf("review history loses before-image: %#v", result)
	}
	result = profileChangePageFromStore(accountdomain.ProfileChangeListResult{
		Items: []accountdomain.ProfileChangeReview{{AccountID: "acc", HasPrevious: false, Change: accountdomain.ProfileChangeRequest{Id: "legacy", Status: "APPROVED", RequestedAt: at}}}, Total: 1,
	}, params)
	if result.Items[0].CurrentKnown {
		t.Fatal("legacy review must NOT pretend current values are the before-image")
	}
}
