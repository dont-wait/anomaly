package composition

import (
	"github.com/dont-wait/anomaly/internal/application/loan/commands"
	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/loanadapter"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/infrastructure/otp"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	loanhttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/loan"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

func AttachLoanProducts(h *loanhttp.Handler, catalog *mongorepo.LoanPackageCatalog) {
	h.Create.PackagePolicy = catalog
	h.Products = loanqueries.NewLoanProductCatalogQueryService(catalog)
	h.PreApplicationQuote = loanqueries.NewPreApplicationQuoteQueryService(catalog)
	h.CatalogSeeder = catalog
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
		DocumentReview: &commands.ReviewLoanDocumentCommandHandler{Repo: repo, Authorizer: access},
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
		Offer:         &commands.LoanOfferCommandHandler{Repo: repo, Authorizer: access, Evidence: evidence},
		ContractEmail: access,
		Disburse: &commands.DisburseLoanCommandHandler{
			Repo:       repo,
			Authorizer: access,
			Evidence:   evidence,
			Bank:       bank,
		},
		Contracts: loanqueries.NewLoanContractQueryService(
			mongorepo.NewLoanContractQuery(read),
		),
		Read:     repo,
		List:     read,
		Evidence: evidence,
		Media:    media,
		Reviewer: access,
	}
}

// AttachLoanContractOTP configures a dedicated loan contract challenge. SMTP
// misconfiguration fails closed via the disabled sender used for transfers.
func AttachLoanContractOTP(h *loanhttp.Handler, rdb *redis.Client, smtp *domain.SMTPConfig, logger zerolog.Logger) {
	if h != nil {
		h.ContractOTP = otp.NewLoanContractOTPStore(rdb, newTransferMailSender(smtp, logger))
	}
}
