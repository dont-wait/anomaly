package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SavingsPackageCatalog exposes the actual persisted Mongo savings_packages
// catalog, validated against the approved simulated domain policy. It seeds
// the default product ONCE, without modifying existing policy records.
type SavingsPackageCatalog struct{ collection *mongodrv.Collection }

type savingsTermRecord struct {
	Months       int             `bson:"months"`
	InterestRate bson.Decimal128 `bson:"interest_rate"`
}

type savingsPackageRecord struct {
	PackageCode string              `bson:"package_code"`
	Version     int                 `bson:"version"`
	Name        string              `bson:"name"`
	Status      string              `bson:"status"`
	Currency    string              `bson:"currency"`
	MinDeposit  int64               `bson:"min_deposit"`
	TermOptions []savingsTermRecord `bson:"term_options"`
	EffectiveAt time.Time           `bson:"effective_at"`
}

func NewSavingsPackageCatalog(client *mongodrv.Client, name string) *SavingsPackageCatalog {
	return &SavingsPackageCatalog{collection: client.Database(name).Collection("savings_packages")}
}

func decimalRate(bps int64) (bson.Decimal128, error) {
	return bson.ParseDecimal128(fmt.Sprintf("%d.%02d", bps/100, bps%100))
}

func (c *SavingsPackageCatalog) EnsureSeeded(ctx context.Context) error {
	if c == nil || c.collection == nil {
		return errors.New("savings catalog unavailable")
	}
	if err := EnsureCatalogIndexes(ctx, c.collection.Database(), c.collection.Name()); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	record := savingsPackageRecord{
		PackageCode: savingsqueries.StandardPackageID, Version: 1,
		Name: "Tiết kiệm có kỳ hạn", Status: "ACTIVE", Currency: "VND",
		MinDeposit: savings.MinimumDepositVND, EffectiveAt: time.Now().UTC(),
	}
	for _, rate := range savings.CurrentTermRates() {
		d, err := decimalRate(rate.AnnualRateBps)
		if err != nil {
			return err
		}
		record.TermOptions = append(record.TermOptions, savingsTermRecord{Months: rate.Months, InterestRate: d})
	}
	_, err := c.collection.UpdateOne(ctx,
		bson.M{"package_code": savingsqueries.StandardPackageID},
		bson.M{"$setOnInsert": record}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("seed savings package: %w", err)
	}
	items, err := c.ListActive(ctx)
	if err != nil {
		return err
	}
	return savingsqueries.ValidateCatalog(items)
}

func (c *SavingsPackageCatalog) ListActive(ctx context.Context) ([]savingsqueries.Package, error) {
	if c == nil || c.collection == nil {
		return nil, errors.New("savings catalog unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := c.collection.Find(ctx, bson.M{"status": "ACTIVE", "effective_at": bson.M{"$lte": time.Now().UTC()}},
		options.Find().SetSort(bson.D{{Key: "package_code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var records []savingsPackageRecord
	if err := cur.All(ctx, &records); err != nil {
		return nil, err
	}
	// Không có bản ACTIVE nghĩa là đang ngừng bán, không phải lỗi.
	items := make([]savingsqueries.Package, 0, len(records))
	for _, r := range records {
		p := savingsqueries.Package{
			PackageID: r.PackageCode, Version: r.Version, Name: r.Name, Currency: r.Currency,
			MinimumDeposit: r.MinDeposit, EffectiveAt: r.EffectiveAt,
		}
		for _, t := range r.TermOptions {
			rateBps, err := rateBpsFromDecimal(t.InterestRate)
			if err != nil {
				return nil, savingsqueries.ErrPackageUnavailable
			}
			label := fmt.Sprintf("%d tháng", t.Months)
			if t.Months%12 == 0 {
				label = fmt.Sprintf("%d năm", t.Months/12)
			}
			p.TermOptions = append(p.TermOptions, savingsqueries.TermOption{
				TermMonths: t.Months, TermLabel: label,
				AnnualInterestRate: savingsqueries.RateString(rateBps),
				AnnualRateBps:      rateBps,
			})
		}
		items = append(items, p)
	}
	if err := savingsqueries.ValidateCatalog(items); err != nil {
		return nil, err
	}
	return items, nil
}
