package notification

import (
	"strings"
	"testing"

	"github.com/dont-wait/anomaly/internal/domain/notification"
)

func TestLoanRepaymentDueTemplate(t *testing.T) {
	tmpl, ok := Lookup(notification.TypeLoanRepaymentDue)
	if !ok || tmpl.Category != notification.CategoryLoan || tmpl.Severity != notification.SeverityWarning {
		t.Fatalf("unexpected loan reminder registration: %+v", tmpl)
	}
	for _, kind := range []string{"T-5", "T-0"} {
		title, body, data, err := tmpl.Render(map[string]string{"kind": kind, "loanId": "loan-test"})
		if err != nil || title == "" || body == "" || data["loanId"] != "loan-test" {
			t.Fatalf("kind %s: title=%s body=%s data=%v err=%v", kind, title, body, data, err)
		}
		if strings.Contains(title+body, "loan-test") || strings.Contains(title+body, "123456") {
			t.Fatal("PII leaked into lock-screen text")
		}
	}
	if _, _, _, err := tmpl.Render(map[string]string{"kind": "T-5"}); err == nil {
		t.Fatal("missing loanId must fail")
	}
	if _, _, _, err := tmpl.Render(map[string]string{"loanId": "loan-test", "kind": "T-9"}); err == nil {
		t.Fatal("unsupported kind must fail")
	}
}
