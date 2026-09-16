package composition

import (
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/transaction/commands"
	"github.com/dont-wait/anomaly/internal/application/transaction/queries"
	handlertx "github.com/dont-wait/anomaly/internal/presentation/http/handler/transaction"
)

type TransactionAccountRepository = commands.AccountRepository
type TransactionFeedRepository = queries.AccountFeedRepository
type TransactionWriter = commands.TransactionWriter

func NewTransactionHandler(
	accounts TransactionAccountRepository,
	writer TransactionWriter,
	feedRepo TransactionFeedRepository,
	logger zerolog.Logger,
) *handlertx.Handler {
	return handlertx.NewHandler(
		logger,
		commands.NewTransferCommandHandler(accounts, writer),
		queries.NewListAccountFeedQueryHandler(feedRepo),
	)
}
