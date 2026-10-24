package composition

import (
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/infrastructure/cache"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	savingshttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/savings"
	"github.com/redis/go-redis/v9"
)

func NewSavingsHandler(catalog *mongo.SavingsPackageCatalog, bank *eventstore.BankRepository, redisClient *redis.Client) *savingshttp.Handler {
	return &savingshttp.Handler{Service: &savingsqueries.Service{
		Catalog: catalog, Accounts: bank, Quotes: cache.NewSavingsQuoteRedis(redisClient),
	}}
}
