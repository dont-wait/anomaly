package composition

import (
	loancommands "github.com/dont-wait/anomaly/internal/application/loan/commands"
	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/infrastructure/cache"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	loanhttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/loan"

	"github.com/redis/go-redis/v9"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

// Gan cac thanh phan tra no vao Loan Handler.
//
// WRITE:
// HTTP -> Command -> Banking EventStoreDB
//
// READ:
// HTTP -> Query -> Redis -> MongoDB
func AttachLoanRepayment(
	h *loanhttp.Handler,
	bank *eventstore.BankRepository,
	loanRepo *eventstore.LoanRepository,
	mongoClient *mongodrv.Client,
	dbName string,
	rdb *redis.Client,
) {
	cacheAdapter := cache.NewLoanRepaymentRedis(rdb)

	queryAdapter := mongorepo.NewLoanRepaymentQuery(
		mongoClient,
		dbName,
	)

	queryService := loanqueries.NewRepaymentQueryService(
		queryAdapter,
		cacheAdapter,
	)

	commandHandler := &loancommands.RepayLoanCommandHandler{
		Repo: loanRepo,
		Bank: bank,
	}

	h.Repayment = &loanhttp.RepaymentHTTP{
		Queries:     queryService,
		Commands:    commandHandler,
		Idempotency: cacheAdapter,
	}
}
