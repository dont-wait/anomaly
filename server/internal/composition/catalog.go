package composition

import (
	appcatalog "github.com/dont-wait/anomaly/internal/application/catalog"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	cataloghttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/catalog"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

// CatalogParts giữ store để tiến trình chạy EnsureIndexes lúc khởi động.
type CatalogParts struct {
	Store *mongo.ProductCatalogStore
	HTTP  *cataloghttp.Handler
}

func NewCatalogHandler(client *mongodrv.Client, dbName string, loanRead *mongo.LoanReadModel, savingsRead *mongo.SavingsRead) *CatalogParts {
	store := mongo.NewProductCatalogStore(client, dbName)
	return &CatalogParts{
		Store: store,
		HTTP: &cataloghttp.Handler{
			Catalog: &appcatalog.Service{Store: store},
			Savings: &savingsqueries.AdminSavingsService{Store: savingsRead},
			Loans:   mongo.NewAdminLoanContracts(loanRead, mongo.NewLoanRepaymentQuery(client, dbName)),
		},
	}
}
