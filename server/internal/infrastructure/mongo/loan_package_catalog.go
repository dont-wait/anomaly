package mongo

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

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
	ID          bson.ObjectID `bson:"_id"`
	Version     int           `bson:"version"`
	Status      string        `bson:"status"`
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

// Package records are written with a single canonical package_code.  Read-side
// normalization tolerates legacy casing in status without accepting unknown values.
const catalogReadTimeout = 5 * time.Second

func (c *LoanPackageCatalog) EnsureIndexes(ctx context.Context) error {
	if c == nil || c.db == nil {
		return fmt.Errorf("loan package catalog is not configured")
	}
	return EnsureCatalogIndexes(ctx, c.db, "loan_packages")
}

// SeedDefaultPackage inserts the development catalog entry when it is absent.
// Existing catalog configuration is deliberately left untouched.
func (c *LoanPackageCatalog) SeedDefaultPackage(ctx context.Context) (bool, error) {
	if c == nil || c.db == nil {
		return false, fmt.Errorf("loan package catalog is not configured")
	}
	const code = "PERSONAL_UNSECURED_SALARY"
	writeCtx, cancel := context.WithTimeout(ctx, catalogReadTimeout)
	defer cancel()
	// Mọi phiên bản của mã này, kể cả đã ngừng bán, đều chặn việc seed lại.
	found, err := c.db.Collection("loan_packages").CountDocuments(writeCtx, bson.M{"package_code": code})
	if err != nil {
		return false, err
	}
	if found > 0 {
		return false, nil
	}
	_, err = c.db.Collection("loan_packages").InsertOne(writeCtx, bson.M{
		"status":       "ACTIVE",
		"version":      1,
		"package_code": code,
		"name":         "Vay tín chấp theo lương",
		"min_amount":   int64(1_000_000),
		"max_amount":   int64(500_000_000),
		"term_options": bson.A{
			bson.M{"months": 6},
			bson.M{"months": 12},
			bson.M{"months": 24},
			bson.M{"months": 36},
			bson.M{"months": 48},
			bson.M{"months": 60},
		},
		"created_at": time.Now().UTC(),
	})
	if mongodrv.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert default loan package: %w", err)
	}
	return true, nil
}

// activePackages đọc mọi bản ACTIVE. Index một phần đảm bảo mỗi mã tối đa một bản,
// phần kiểm tra trùng chỉ phòng dữ liệu chưa qua EnsureIndexes.
func (c *LoanPackageCatalog) activePackages(ctx context.Context) ([]loanPackageRecord, error) {
	if c == nil || c.db == nil {
		return nil, fmt.Errorf("loan package catalog is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, catalogReadTimeout)
	defer cancel()
	cur, err := c.db.Collection("loan_packages").Find(ctx, bson.M{"status": "ACTIVE"},
		options.Find().SetSort(bson.D{{Key: "package_code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var records []loanPackageRecord
	if err := cur.All(ctx, &records); err != nil {
		return nil, err
	}
	for i := 1; i < len(records); i++ {
		if records[i].PackageCode == records[i-1].PackageCode {
			return nil, fmt.Errorf("duplicate active loan package %q; reconcile data before accepting loans", records[i].PackageCode)
		}
	}
	return records, nil
}

// activePackageByCode dùng khi chiếu khoản vay để gắn hợp đồng với gói đang bán.
func (c *LoanPackageCatalog) activePackageByCode(ctx context.Context, code string) (*loanPackageRecord, error) {
	records, err := c.activePackages(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].PackageCode == code {
			return &records[i], nil
		}
	}
	return nil, nil
}

func (r loanPackageRecord) toPackage() (loanqueries.LoanPackage, error) {
	code := r.PackageCode
	minAmount, err := packageMoneyToInt64(r.MinAmount)
	if err != nil {
		return loanqueries.LoanPackage{}, fmt.Errorf("loan package %s min_amount: %w", code, err)
	}
	maxAmount, err := packageMoneyToInt64(r.MaxAmount)
	if err != nil {
		return loanqueries.LoanPackage{}, fmt.Errorf("loan package %s max_amount: %w", code, err)
	}
	if minAmount <= 0 || maxAmount < minAmount || strings.TrimSpace(r.Name) == "" || len(r.TermOptions) == 0 {
		return loanqueries.LoanPackage{}, fmt.Errorf("loan package %s contains invalid catalog data", code)
	}
	terms := make([]int, 0, len(r.TermOptions))
	seen := map[int]bool{}
	for _, term := range r.TermOptions {
		switch term.Months {
		case 6, 12, 24, 36, 48, 60:
		default:
			return loanqueries.LoanPackage{}, fmt.Errorf("loan package %s has unsupported term %d", code, term.Months)
		}
		if !seen[term.Months] {
			terms = append(terms, term.Months)
			seen[term.Months] = true
		}
	}
	return loanqueries.LoanPackage{PackageCode: code, Version: r.Version, Name: r.Name, MinAmount: minAmount, MaxAmount: maxAmount, TermMonths: terms}, nil
}

func (c *LoanPackageCatalog) ListActiveLoanPackages(ctx context.Context) ([]loanqueries.LoanPackage, error) {
	records, err := c.activePackages(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]loanqueries.LoanPackage, 0, len(records))
	for _, record := range records {
		item, err := record.toPackage()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ValidateLoanPackage is called by the CreateLoanApplication COMMAND before
// the initial EventStoreDB append, not just by the HTTP handler or Quote.
func (c *LoanPackageCatalog) ValidateLoanPackage(ctx context.Context, code string, amount int64, months int) (int, error) {
	items, err := c.ListActiveLoanPackages(ctx)
	if err != nil {
		return 0, fmt.Errorf("loan package catalog unavailable: %w", err)
	}
	for _, item := range items {
		if item.PackageCode != code {
			continue
		}
		if amount < item.MinAmount || amount > item.MaxAmount {
			return 0, fmt.Errorf("loan amount outside active package limits")
		}
		for _, term := range item.TermMonths {
			if term == months {
				return item.Version, nil
			}
		}
		return 0, fmt.Errorf("loan term not supported by active package")
	}
	return 0, fmt.Errorf("loan package not active or not found")
}
