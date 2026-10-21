package mongo

import (
	"context"
	"fmt"
	"math/big"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// LoanPackageCatalog chỉ đọc các sản phẩm có thể đăng ký qua POST /api/loans.
// Lãi suất phải tính theo Domain khi có thu nhập/khu vực/kỳ hạn thực tế.
type LoanPackageCatalog struct{ db *mongodrv.Database }

func NewLoanPackageCatalog(client *mongodrv.Client, dbName string) *LoanPackageCatalog {
	return &LoanPackageCatalog{db: client.Database(dbName)}
}

type loanPackageRecord struct {
	PackageCode string        `bson:"package_code"`
	Name        string        `bson:"name"`
	MinAmount   bson.RawValue `bson:"min_amount"`
	MaxAmount   bson.RawValue `bson:"max_amount"`
	TermOptions []struct {
		Months int `bson:"months"`
	} `bson:"term_options"`
}

func packageMoneyToInt64(v bson.RawValue) (int64, error) {
	switch v.Type {
	case bson.TypeInt64:
		return v.Int64(), nil
	case bson.TypeInt32:
		return int64(v.Int32()), nil
	case bson.TypeDecimal128:
		d := v.Decimal128()
		rational, ok := new(big.Rat).SetString(d.String())
		if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
			return 0, fmt.Errorf("invalid integer amount in loan package")
		}
		return rational.Num().Int64(), nil
	default:
		return 0, fmt.Errorf("unsupported loan package amount type %v", v.Type)
	}
}

func (c *LoanPackageCatalog) ListActiveLoanPackages(ctx context.Context) ([]loanqueries.LoanPackage, error) {
	if c == nil || c.db == nil {
		return nil, fmt.Errorf("loan package MongoDB reader is not configured")
	}

	// MVP chỉ công bố sản phẩm mà Create Loan Application thực sự hỗ trợ.
	cursor, err := c.db.Collection("loan_packages").Find(ctx, bson.M{
		"package_code": "PERSONAL_UNSECURED_SALARY",
		"status":       bson.M{"$in": bson.A{"active", "ACTIVE"}},
	}, options.Find().SetSort(bson.D{{Key: "package_code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := make([]loanqueries.LoanPackage, 0)
	for cursor.Next(ctx) {
		var doc loanPackageRecord
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		minAmount, err := packageMoneyToInt64(doc.MinAmount)
		if err != nil {
			return nil, fmt.Errorf("loan package %s min_amount: %w", doc.PackageCode, err)
		}
		maxAmount, err := packageMoneyToInt64(doc.MaxAmount)
		if err != nil {
			return nil, fmt.Errorf("loan package %s max_amount: %w", doc.PackageCode, err)
		}
		if minAmount <= 0 || maxAmount < minAmount || doc.Name == "" || len(doc.TermOptions) == 0 {
			return nil, fmt.Errorf("loan package %s contains invalid catalog data", doc.PackageCode)
		}
		terms := make([]int, 0, len(doc.TermOptions))
		seen := map[int]bool{}
		for _, term := range doc.TermOptions {
			// Đồng bộ kỳ hạn hợp lệ với Domain, không công bố kỳ hạn không được hỗ trợ.
			switch term.Months {
			case 6, 12, 24, 36, 48, 60:
			default:
				return nil, fmt.Errorf("loan package %s has unsupported term %d", doc.PackageCode, term.Months)
			}
			if !seen[term.Months] {
				terms = append(terms, term.Months)
				seen[term.Months] = true
			}
		}
		items = append(items, loanqueries.LoanPackage{
			PackageCode: doc.PackageCode, Name: doc.Name, MinAmount: minAmount, MaxAmount: maxAmount, TermMonths: terms,
		})
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
