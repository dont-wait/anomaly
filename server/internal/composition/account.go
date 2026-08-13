package composition

import (
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	handleraccount "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
)

type AccountRepository interface {
	commands.AccountRepository
	queries.AccountQueryRepository
}

func NewAccountHandler(repo AccountRepository, logger zerolog.Logger) *handleraccount.Handler {
	return handleraccount.NewHandler(
		logger,
		commands.NewCreateAccountCommandHandler(repo),
		queries.NewGetAccountByIDQueryHandler(repo),
		queries.NewGetAccountByEmailQueryHandler(repo),
		queries.NewGetAllAccountsQueryHandler(repo),
	)
}
