package loan

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServiceErrorMapsTypedGRPCUnavailable(t *testing.T) {
	cases := []struct {
		code codes.Code
		want int
	}{
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		serviceError(w, status.Error(tc.code, "backend offline"))
		if w.Code != tc.want {
			t.Fatalf("code %v mapped to %d, want %d", tc.code, w.Code, tc.want)
		}
	}
}
func TestOfferMutationRolloutGate(t *testing.T) {
	t.Setenv("LOAN_OFFER_WRITES_ENABLED", "")
	if offerWritesEnabled() {
		t.Fatal("unsafe: writes enabled by default")
	}
	t.Setenv("LOAN_OFFER_WRITES_ENABLED", "true")
	if !offerWritesEnabled() {
		t.Fatal("writes not enabled after rollout")
	}
}
