package composition

import (
	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	handleraccount "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
)

type AccountRepository interface {
	commands.AccountRepository
	queries.AccountQueryRepository
}

func NewAccountHandler(repo AccountRepository) *handleraccount.Handler {
	return handleraccount.NewHandler(
		commands.NewCreateAccountCommandHandler(repo),
		queries.NewGetAccountByIDQueryHandler(repo),
		queries.NewGetAccountByEmailQueryHandler(repo),
		queries.NewGetAllAccountsQueryHandler(repo),
	)
}
