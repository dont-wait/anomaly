package loan

import (
	"testing"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
)

func TestRepaymentHistoryKeysetDoesNotShift(t *testing.T) {
	makePayments := func(nums ...int) []loanqueries.Payment {
		items := make([]loanqueries.Payment, 0, len(nums))
		for _, n := range nums {
			items = append(items, loanqueries.Payment{InstallmentNo: n})
		}
		return items
	}
	page, cursor := repaymentHistoryPage(makePayments(5, 4, 3, 2, 1), 0, 2)
	if len(page) != 2 || page[0].InstallmentNo != 5 || page[1].InstallmentNo != 4 || cursor == nil || *cursor != "4" {
		t.Fatalf("first page: %+v %v", page, cursor)
	}
	page, cursor = repaymentHistoryPage(makePayments(6, 5, 4, 3, 2, 1), 4, 2)
	if len(page) != 2 || page[0].InstallmentNo != 3 || page[1].InstallmentNo != 2 || cursor == nil || *cursor != "2" {
		t.Fatalf("second page shifted after new payment: %+v %v", page, cursor)
	}
	page, cursor = repaymentHistoryPage(makePayments(6, 5, 4, 3, 2, 1), 2, 2)
	if len(page) != 1 || page[0].InstallmentNo != 1 || cursor != nil {
		t.Fatalf("last page: %+v %v", page, cursor)
	}
}
