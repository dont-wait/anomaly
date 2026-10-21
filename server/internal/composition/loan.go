package composition

import (
	"github.com/dont-wait/anomaly/internal/application/loan/commands"
	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/loanadapter"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	loanhttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/loan"
)

func AttachLoanProducts(h *loanhttp.Handler, catalog *mongorepo.LoanPackageCatalog) {
	h.Create.PackagePolicy = catalog
	h.Products = loanqueries.NewLoanProductCatalogQueryService(catalog)
	h.PreApplicationQuote = loanqueries.NewPreApplicationQuoteQueryService(catalog)
}

func NewLoanHandler(
	bank *eventstore.BankRepository,
	repo *eventstore.LoanRepository,
	read *mongorepo.LoanReadModel,
	evidence *loanadapter.EvidenceStore,
	media *rustfs.MediaRepository,
) *loanhttp.Handler {
	access := &loanadapter.Access{
		Accounts: bank,
	}

	verifier := &loanadapter.DocumentVerifier{
		Media: media,
	}

	return &loanhttp.Handler{
		Create: commands.NewCreateLoanApplicationCommandHandler(
			repo,
			access,
		),
		Add: commands.NewAddLoanDocumentCommandHandler(
			repo,
			verifier,
		),
		Submit: commands.NewSubmitLoanApplicationCommandHandler(
			repo,
		),
		Review: commands.NewStartLoanReviewCommandHandler(
			repo,
			access,
		),
		Assess: commands.NewAssessLoanApplicationCommandHandler(
			repo,
			access,
			&loanadapter.AssessmentProvider{
				Evidence: evidence,
			},
		),
		Decide: commands.NewManualLoanDecisionCommandHandler(
			repo,
			access,
		),
		Disburse: &commands.DisburseLoanCommandHandler{
			Repo:       repo,
			Authorizer: access,
			Evidence:   evidence,
			Bank:       bank,
		},
		Read:     repo,
		List:     read,
		Evidence: evidence,
		Media:    media,
		Reviewer: access,
	}
}
