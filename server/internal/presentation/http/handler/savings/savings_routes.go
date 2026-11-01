package savings

import (
	"net/http"

	accountqueries "github.com/dont-wait/anomaly/internal/application/account/queries"
	savingscommands "github.com/dont-wait/anomaly/internal/application/savings/commands"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

// RegisterRoutes exposes savings quotes and atomic banking commands.
func RegisterRoutes(router *openapi.Registry, h *Handler, tokens accountqueries.TokenService) {
	if router == nil || h == nil || tokens == nil {
		return
	}
	auth := func(pattern string, fn http.HandlerFunc, op openapi.Operation) {
		op.Auth = true
		op.Tags = []string{"Savings"}
		op.FailureResponses = map[int]any{}
		for _, status := range op.FailureStatuses {
			op.FailureResponses[status] = apiError{}
		}
		router.Handle(pattern, middleware.RequireAuth(tokens)(fn), op)
	}
	auth("GET /api/savings/terms", h.terms, openapi.Operation{
		ID: "getSavingsTerms", Summary: "Read current savings product conditions before opening",
		Description: "Returns the current product conditions. The quote determines the concrete amount, interest rate, and maturity for each opening.",
		Response:    TermsSnapshot{}, FailureStatuses: []int{401, 503},
	})
	auth("GET /api/savings/packages", h.packages, openapi.Operation{
		ID: "listSavingsPackages", Summary: "List approved VND savings terms and interest rates",
		Response: struct {
			Items []savingsqueries.Package `json:"items"`
		}{},
		FailureStatuses: []int{401, 503},
	})
	auth("POST /api/savings/quotes", h.quote, openapi.Operation{
		ID: "createSavingsQuote", Summary: "Preview savings interest without debiting money",
		Description: "The quote expires 5 minutes after creation and does not reserve funds. The opening command rechecks policy, ownership and balance.",
		Request:     savingsqueries.CreateQuoteInput{}, Response: savingsqueries.QuoteResponse{},
		FailureStatuses: []int{400, 401, 404, 422, 503},
	})
	auth("POST /api/savings/contracts", h.openContract, openapi.Operation{ID: "openSavingsContract", Summary: "Confirm quote and atomically open savings", SuccessStatus: http.StatusCreated, Parameters: []openapi.Parameter{{Name: "Idempotency-Key", In: "header", Required: true, Type: "string", Format: "uuid"}}, Request: savingscommands.OpenInput{}, Response: savingscommands.OpenResponse{}, FailureStatuses: []int{400, 401, 404, 409, 410, 422, 503}})
	auth("POST /api/savings/contracts/{id}/withdrawal-quotes", h.withdrawalQuote, openapi.Operation{ID: "quoteSavingsWithdrawal", Summary: "Preview full maturity payout", Request: struct {
		WithdrawalType string `json:"withdrawalType"`
	}{}, Response: savingscommands.WithdrawalPreviewResponse{}, FailureStatuses: []int{400, 401, 404, 409, 422, 423, 503}})
	auth("POST /api/savings/contracts/{id}/withdrawals", h.withdraw, openapi.Operation{ID: "withdrawSavings", Summary: "Settle matured savings exactly once", Parameters: []openapi.Parameter{{Name: "Idempotency-Key", In: "header", Required: true, Type: "string", Format: "uuid"}}, Request: savingscommands.WithdrawInput{}, Response: savingscommands.WithdrawResponse{}, FailureStatuses: []int{400, 401, 404, 409, 410, 422, 423, 503}})
	h.RegisterReadRoutes(auth)
	auth("GET /api/savings/contracts/{id}/settlement", h.settlement, openapi.Operation{ID: "savingsSettlementReceipt", Summary: "Read authoritative settlement receipt", Response: savingscommands.SettlementReceipt{}, FailureStatuses: []int{401, 404, 409, 503}})
}
