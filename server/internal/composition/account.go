package composition

import (
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/kyc"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	handleraccount "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
)

// AccountRepository gộp cả commands.AccountRepository (Save, Find*)
// và queries.AccountQueryRepository (Find*) để composition không phải
// ép kiểu khi truyền cùng 1 repo cho nhiều use case.
type AccountRepository interface {
	commands.AccountRepository
	commands.AccountCreator
	queries.AccountQueryRepository
}

func NewAccountHandler(repo AccountRepository, tokenSvc *auth.TokenService, logger zerolog.Logger, recorders ...audit.Recorder) *handleraccount.Handler {
	return newAccountHandler(repo, tokenSvc, nil, nil, logger, recorders...)
}

func NewAccountHandlerWithKYC(repo AccountRepository, tokenSvc *auth.TokenService, kycClient *kyc.Client, mediaRepo *rustfs.MediaRepository, logger zerolog.Logger, recorders ...audit.Recorder) *handleraccount.Handler {
	return newAccountHandler(repo, tokenSvc, kycClient, mediaRepo, logger, recorders...)
}

func newAccountHandler(repo AccountRepository, tokenSvc *auth.TokenService, kycClient *kyc.Client, mediaRepo *rustfs.MediaRepository, logger zerolog.Logger, recorders ...audit.Recorder) *handleraccount.Handler {
	login := queries.NewLoginQueryHandler(repo, tokenSvc)
	if len(recorders) > 0 {
		login.WithAudit(recorders[0])
	}
	return handleraccount.NewHandler(
		logger,
		commands.NewRegisterAccountCommandHandler(repo, repo),
		commands.NewVerifyAccountCommandHandler(repo),
		login,
		queries.NewGetAccountByIDQueryHandler(repo),
		queries.NewGetAccountByEmailQueryHandler(repo),
		queries.NewGetAllAccountsQueryHandler(repo),
		tokenSvc,
		kycClient,
		mediaRepo,
	)
}
