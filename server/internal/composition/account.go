package composition

import (
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
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

func NewAccountHandler(repo AccountRepository,
	tokenSvc *auth.TokenService,
	logger zerolog.Logger,
) *handleraccount.Handler {
	return handleraccount.NewHandler(
		logger,
		commands.NewRegisterAccountCommandHandler(repo, repo),
		commands.NewVerifyAccountCommandHandler(repo),
		queries.NewLoginQueryHandler(repo, tokenSvc),
		queries.NewGetAccountByIDQueryHandler(repo),
		queries.NewGetAccountByEmailQueryHandler(repo),
		queries.NewGetAllAccountsQueryHandler(repo),
	)
}
