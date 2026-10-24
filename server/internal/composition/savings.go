package composition

import (
	savingscommands "github.com/dont-wait/anomaly/internal/application/savings/commands"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/infrastructure/cache"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	savingshttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/savings"
	"github.com/redis/go-redis/v9"
)

func NewSavingsHandler(catalog *mongo.SavingsPackageCatalog, bank *eventstore.BankRepository, redisClient *redis.Client, read ...*mongo.SavingsRead) *savingshttp.Handler {
	q := &savingsqueries.Service{Catalog: catalog, Accounts: bank, Quotes: cache.NewSavingsQuoteRedis(redisClient)}
	handler := &savingshttp.Handler{Service: q, Commands: &savingscommands.Service{Quotes: q, Bank: savingsBankCommandAdapter{BankRepository: bank}, WithdrawalQuotes: cache.NewSavingsWithdrawalQuoteRedis(redisClient)}}
	if len(read) > 0 && read[0] != nil {
		handler.Read = &savingsqueries.SavingsReadService{Store: read[0], Accounts: read[0], Cache: cache.NewSavingsReadRedis(redisClient)}
	}
	return handler
}
