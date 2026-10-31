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
	ctx, cancel := context.WithTimeout(ctx, catalogReadTimeout)
	defer cancel()
	_, err := c.db.Collection("loan_packages").Indexes().CreateOne(ctx,
		mongodrv.IndexModel{Keys: bson.D{{Key: "package_code", Value: 1}}, Options: options.Index().SetUnique(true)},
	)
	if err != nil {
		return fmt.Errorf("loan_packages contains duplicate package_code or unique index failed: %w", err)
	}
	return nil
}

// SeedDefaultPackage inserts the development catalog entry when it is absent.
// Existing catalog configuration is deliberately left untouched.
func (c *LoanPackageCatalog) SeedDefaultPackage(ctx context.Context) (bool, error) {
	if c == nil || c.db == nil {
		return false, fmt.Errorf("loan package catalog is not configured")
	}
	const code = "PERSONAL_UNSECURED_SALARY"
	existing, err := c.uniquePackageByCode(ctx, code)
	if err != nil {
		return false, err
	}
	if existing != nil {
		return false, nil
	}
	_, err = c.db.Collection("loan_packages").InsertOne(ctx, bson.M{
		"status":       "ACTIVE",
		"package_code": code,
		"name":         "Vay tín chấp theo lương",
		"min_amount":   int64(1_000_000),
		"max_amount":   int64(500_000_000),
		"term_options": bson.A{
			bson.M{"months": 6}, bson.M{"months": 12}, bson.M{"months": 24},
			bson.M{"months": 36}, bson.M{"months": 48}, bson.M{"months": 60},
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

// uniquePackageByCode detects conflicting active/inactive versions instead of
// relying on FindOne or a nondeterministic cursor ordering.
func (c *LoanPackageCatalog) uniquePackageByCode(ctx context.Context, code string) (*loanPackageRecord, error) {
	if c == nil || c.db == nil {
		return nil, fmt.Errorf("loan package catalog is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, catalogReadTimeout)
	defer cancel()
	cur, err := c.db.Collection("loan_packages").Find(ctx, bson.M{"package_code": code})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var one *loanPackageRecord
	for cur.Next(ctx) {
		if one != nil {
			return nil, fmt.Errorf("duplicate loan package code %q; reconcile data before accepting loans", code)
		}
		var item loanPackageRecord
		if err := cur.Decode(&item); err != nil {
			return nil, err
		}
		one = &item
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return one, nil
}

func normalizePackageStatus(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func (c *LoanPackageCatalog) ListActiveLoanPackages(ctx context.Context) ([]loanqueries.LoanPackage, error) {
	// MVP exposes only the product accepted by the create application API.
	const code = "PERSONAL_UNSECURED_SALARY"
	record, err := c.uniquePackageByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if record == nil || normalizePackageStatus(record.Status) != "ACTIVE" {
		return []loanqueries.LoanPackage{}, nil
	}
	minAmount, err := packageMoneyToInt64(record.MinAmount)
	if err != nil {
		return nil, fmt.Errorf("loan package %s min_amount: %w", code, err)
	}
	maxAmount, err := packageMoneyToInt64(record.MaxAmount)
	if err != nil {
		return nil, fmt.Errorf("loan package %s max_amount: %w", code, err)
	}
	if minAmount <= 0 || maxAmount < minAmount || strings.TrimSpace(record.Name) == "" || len(record.TermOptions) == 0 {
		return nil, fmt.Errorf("loan package %s contains invalid catalog data", code)
	}
	terms := make([]int, 0, len(record.TermOptions))
	seen := map[int]bool{}
	for _, term := range record.TermOptions {
		switch term.Months {
		case 6, 12, 24, 36, 48, 60:
		default:
			return nil, fmt.Errorf("loan package %s has unsupported term %d", code, term.Months)
		}
		if !seen[term.Months] {
			terms = append(terms, term.Months)
			seen[term.Months] = true
		}
	}
	return []loanqueries.LoanPackage{{PackageCode: record.PackageCode, Name: record.Name, MinAmount: minAmount, MaxAmount: maxAmount, TermMonths: terms}}, nil
}

// ValidateLoanPackage is called by the CreateLoanApplication COMMAND before
// the initial EventStoreDB append, not just by the HTTP handler or Quote.
func (c *LoanPackageCatalog) ValidateLoanPackage(ctx context.Context, code string, amount int64, months int) error {
	items, err := c.ListActiveLoanPackages(ctx)
	if err != nil {
		return fmt.Errorf("loan package catalog unavailable: %w", err)
	}
	for _, item := range items {
		if item.PackageCode != code {
			continue
		}
		if amount < item.MinAmount || amount > item.MaxAmount {
			return fmt.Errorf("loan amount outside active package limits")
		}
		for _, term := range item.TermMonths {
			if term == months {
				return nil
			}
		}
		return fmt.Errorf("loan term not supported by active package")
	}
	return fmt.Errorf("loan package not active or not found")
}
