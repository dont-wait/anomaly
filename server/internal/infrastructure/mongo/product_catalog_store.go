package mongo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	appcatalog "github.com/dont-wait/anomaly/internal/application/catalog"
	domain "github.com/dont-wait/anomaly/internal/domain/catalog"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	catalogTimeout         = 10 * time.Second
	catalogAuditCollection = "product_catalog_audit"
	savingsTermsProduct    = "savings_standard"
)

// ProductCatalogStore ghi các phiên bản gói. Mỗi thay đổi trạng thái đi cùng bản
// ghi audit trong một transaction trên replica set.
type ProductCatalogStore struct {
	client *mongodrv.Client
	db     *mongodrv.Database
}

func NewProductCatalogStore(client *mongodrv.Client, dbName string) *ProductCatalogStore {
	return &ProductCatalogStore{client: client, db: client.Database(dbName)}
}

func catalogCollectionName(kind domain.Kind) string {
	if kind == domain.KindSavings {
		return "savings_packages"
	}
	return "loan_packages"
}

// EnsureCatalogIndexes chuyển collection sang mô hình phiên bản. Hàm chạy lại an
// toàn, và làm ở đây vì collection có thể chưa tồn tại khi chạy migration.
func EnsureCatalogIndexes(ctx context.Context, db *mongodrv.Database, name string) error {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	collection := db.Collection(name)
	if _, err := collection.UpdateMany(ctx, bson.M{"version": bson.M{"$exists": false}}, mongodrv.Pipeline{
		{{Key: "$set", Value: bson.M{"version": 1, "status": bson.M{"$toUpper": "$status"}}}},
	}); err != nil {
		return fmt.Errorf("backfill %s versions: %w", name, err)
	}
	if err := collection.Indexes().DropOne(ctx, "package_code_1"); err != nil && !isMissingIndex(err) {
		return fmt.Errorf("drop legacy %s index: %w", name, err)
	}
	_, err := collection.Indexes().CreateMany(ctx, []mongodrv.IndexModel{
		{Keys: bson.D{{Key: "package_code", Value: 1}, {Key: "version", Value: 1}}, Options: options.Index().SetName("uniq_package_version").SetUnique(true)},
		{Keys: bson.D{{Key: "package_code", Value: 1}}, Options: options.Index().SetName("uniq_package_active").SetUnique(true).SetPartialFilterExpression(bson.M{"status": "ACTIVE"})},
		{Keys: bson.D{{Key: "package_code", Value: 1}, {Key: "status", Value: 1}}, Options: options.Index().SetName("uniq_package_draft").SetUnique(true).SetPartialFilterExpression(bson.M{"status": "DRAFT"})},
	})
	if err != nil {
		return fmt.Errorf("%s contains conflicting versions or index failed: %w", name, err)
	}
	return nil
}

func isMissingIndex(err error) bool {
	var command mongodrv.CommandError
	// 26 NamespaceNotFound, 27 IndexNotFound.
	return errors.As(err, &command) && (command.Code == 26 || command.Code == 27)
}

// EnsureIndexes chuẩn bị audit và index cho thống kê sử dụng.
func (s *ProductCatalogStore) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	if _, err := s.db.Collection(catalogAuditCollection).Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys: bson.D{{Key: "kind", Value: 1}, {Key: "code", Value: 1}, {Key: "at", Value: -1}}, Options: options.Index().SetName("catalog_audit_package"),
	}); err != nil {
		return err
	}
	_, err := s.db.Collection("savings_contracts").Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys: bson.D{{Key: "package_id", Value: 1}, {Key: "status", Value: 1}}, Options: options.Index().SetName("savings_package_usage"),
	})
	return err
}

type catalogTermRow struct {
	Months       int             `bson:"months"`
	InterestRate bson.Decimal128 `bson:"interest_rate"`
}

type catalogRow struct {
	PackageCode  string           `bson:"package_code"`
	Version      int              `bson:"version"`
	Status       string           `bson:"status"`
	Name         string           `bson:"name"`
	Description  string           `bson:"description"`
	MinAmount    bson.RawValue    `bson:"min_amount"`
	MaxAmount    bson.RawValue    `bson:"max_amount"`
	MinDeposit   bson.RawValue    `bson:"min_deposit"`
	Terms        []catalogTermRow `bson:"term_options"`
	CreatedBy    string           `bson:"created_by"`
	CreatedAt    *time.Time       `bson:"created_at"`
	PublishedBy  string           `bson:"published_by"`
	PublishedAt  *time.Time       `bson:"published_at"`
	RetiredBy    string           `bson:"retired_by"`
	RetiredAt    *time.Time       `bson:"retired_at"`
	RetireReason string           `bson:"retire_reason"`
}

// rateBpsFromDecimal đổi %/năm trong Mongo sang bps; từ chối giá trị lẻ dưới 0,01%.
func rateBpsFromDecimal(value bson.Decimal128) (int64, error) {
	rate, ok := new(big.Rat).SetString(value.String())
	if !ok {
		return 0, fmt.Errorf("invalid interest rate")
	}
	rate.Mul(rate, big.NewRat(100, 1))
	if !rate.IsInt() || !rate.Num().IsInt64() {
		return 0, fmt.Errorf("interest rate has more than 2 decimals")
	}
	return rate.Num().Int64(), nil
}

func (r catalogRow) toDomain(kind domain.Kind) (domain.Package, error) {
	p := domain.Package{
		Kind: kind, Code: r.PackageCode, Version: r.Version, Status: domain.Status(r.Status),
		Name: r.Name, Description: r.Description, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		PublishedBy: r.PublishedBy, PublishedAt: r.PublishedAt,
		RetiredBy: r.RetiredBy, RetiredAt: r.RetiredAt, RetireReason: r.RetireReason,
		Terms: make([]domain.TermOption, 0, len(r.Terms)),
	}
	var err error
	if kind == domain.KindSavings {
		if p.MinAmount, err = packageMoneyToInt64(r.MinDeposit); err != nil {
			return p, err
		}
	} else {
		if p.MinAmount, err = packageMoneyToInt64(r.MinAmount); err != nil {
			return p, err
		}
		if p.MaxAmount, err = packageMoneyToInt64(r.MaxAmount); err != nil {
			return p, err
		}
	}
	for _, term := range r.Terms {
		option := domain.TermOption{Months: term.Months}
		if kind == domain.KindSavings {
			if option.AnnualRateBps, err = rateBpsFromDecimal(term.InterestRate); err != nil {
				return p, err
			}
		}
		p.Terms = append(p.Terms, option)
	}
	return p, nil
}

func catalogDocument(p domain.Package) (bson.M, error) {
	terms := bson.A{}
	for _, term := range p.Terms {
		row := bson.M{"months": term.Months}
		if p.Kind == domain.KindSavings {
			rate, err := decimalRate(term.AnnualRateBps)
			if err != nil {
				return nil, err
			}
			row["interest_rate"] = rate
		}
		terms = append(terms, row)
	}
	doc := bson.M{
		"package_code": p.Code, "version": p.Version, "status": string(p.Status),
		"name": p.Name, "description": p.Description, "term_options": terms,
		"created_by": p.CreatedBy, "created_at": p.CreatedAt,
	}
	if p.Kind == domain.KindSavings {
		doc["min_deposit"] = p.MinAmount
		doc["currency"] = "VND"
	} else {
		doc["min_amount"] = p.MinAmount
		doc["max_amount"] = p.MaxAmount
	}
	return doc, nil
}

func (s *ProductCatalogStore) find(ctx context.Context, kind domain.Kind, filter bson.M) ([]domain.Package, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	filter["deleted_at"] = bson.M{"$exists": false}
	cur, err := s.db.Collection(catalogCollectionName(kind)).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "package_code", Value: 1}, {Key: "version", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var rows []catalogRow
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	items := make([]domain.Package, 0, len(rows))
	for _, row := range rows {
		p, err := row.toDomain(kind)
		if err != nil {
			return nil, fmt.Errorf("package %s v%d: %w", row.PackageCode, row.Version, err)
		}
		items = append(items, p)
	}
	return items, nil
}

func (s *ProductCatalogStore) Versions(ctx context.Context, kind domain.Kind, code string) ([]domain.Package, error) {
	return s.find(ctx, kind, bson.M{"package_code": code})
}

func (s *ProductCatalogStore) ListAll(ctx context.Context, kind domain.Kind) ([]domain.Package, error) {
	return s.find(ctx, kind, bson.M{})
}

func (s *ProductCatalogStore) InsertDraft(ctx context.Context, draft domain.Package) error {
	doc, err := catalogDocument(draft)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	_, err = s.db.Collection(catalogCollectionName(draft.Kind)).InsertOne(ctx, doc)
	if mongodrv.IsDuplicateKeyError(err) {
		return appcatalog.ErrDraftExists
	}
	return err
}

func (s *ProductCatalogStore) ReplaceDraft(ctx context.Context, draft domain.Package) error {
	doc, err := catalogDocument(draft)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	result, err := s.db.Collection(catalogCollectionName(draft.Kind)).ReplaceOne(ctx,
		bson.M{"package_code": draft.Code, "version": draft.Version, "status": "DRAFT"}, doc)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return appcatalog.ErrNoDraft
	}
	return nil
}

func (s *ProductCatalogStore) DeleteDraft(ctx context.Context, kind domain.Kind, code string) error {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	result, err := s.db.Collection(catalogCollectionName(kind)).DeleteOne(ctx, bson.M{"package_code": code, "status": "DRAFT"})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return appcatalog.ErrNoDraft
	}
	return nil
}

func (s *ProductCatalogStore) DeletePackage(ctx context.Context, kind domain.Kind, code, actorID string, at time.Time) error {
	collection := s.db.Collection(catalogCollectionName(kind))
	live := bson.M{"package_code": code, "deleted_at": bson.M{"$exists": false}}
	return s.transaction(ctx, func(sc context.Context) error {
		// Kiểm tra lại trong transaction để không xóa gói vừa được phát hành.
		active, err := collection.CountDocuments(sc, bson.M{"package_code": code, "status": "ACTIVE", "deleted_at": bson.M{"$exists": false}})
		if err != nil {
			return err
		}
		if active > 0 {
			return appcatalog.ErrPackageActive
		}
		removed, err := collection.DeleteOne(sc, bson.M{"package_code": code, "status": "DRAFT", "deleted_at": bson.M{"$exists": false}})
		if err != nil {
			return err
		}
		hidden, err := collection.UpdateMany(sc, live, bson.M{"$set": bson.M{"deleted_at": at, "deleted_by": actorID}})
		if err != nil {
			return err
		}
		if removed.DeletedCount == 0 && hidden.MatchedCount == 0 {
			return appcatalog.ErrPackageNotFound
		}
		_, err = s.db.Collection(catalogAuditCollection).InsertOne(sc, bson.M{
			"kind": string(kind), "code": code, "action": "delete", "version": "", "actor_id": actorID, "at": at,
		})
		return err
	})
}

// auditID gắn với khóa idempotency nên retry không ghi thêm.
func auditID(t appcatalog.Transition, action string) string {
	sum := sha256.Sum256([]byte(string(t.Kind) + ":" + t.Code + ":" + action + ":" + t.IdempotencyKey))
	return hex.EncodeToString(sum[:])
}

func (s *ProductCatalogStore) transaction(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	session, err := s.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(sc context.Context) (any, error) {
		return nil, fn(sc)
	})
	return err
}

func (s *ProductCatalogStore) writeAudit(ctx context.Context, id string, t appcatalog.Transition, action string, before, after int) error {
	_, err := s.db.Collection(catalogAuditCollection).InsertOne(ctx, bson.M{
		"_id": id, "kind": string(t.Kind), "code": t.Code, "action": action,
		"version": fmt.Sprint(after), "before_version": before, "actor_id": t.ActorID, "reason": t.Reason, "at": t.At,
	})
	return err
}

// replayed trả về phiên bản đã ghi nếu khóa idempotency này từng được dùng.
func (s *ProductCatalogStore) replayed(ctx context.Context, id string, kind domain.Kind, code string) (domain.Package, bool, error) {
	var audit struct {
		Version string `bson:"version"`
	}
	err := s.db.Collection(catalogAuditCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&audit)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return domain.Package{}, false, nil
	}
	if err != nil {
		return domain.Package{}, false, err
	}
	var version int
	if _, err := fmt.Sscan(audit.Version, &version); err != nil {
		return domain.Package{}, false, err
	}
	items, err := s.find(ctx, kind, bson.M{"package_code": code, "version": version})
	if err != nil || len(items) != 1 {
		return domain.Package{}, false, errors.Join(err, errors.New("audited package version missing"))
	}
	return items[0], true, nil
}

func (s *ProductCatalogStore) Publish(ctx context.Context, t appcatalog.Transition) (domain.Package, error) {
	id := auditID(t, "publish")
	collection := s.db.Collection(catalogCollectionName(t.Kind))
	if p, found, err := s.replayed(ctx, id, t.Kind, t.Code); err != nil || found {
		return p, err
	}
	err := s.transaction(ctx, func(sc context.Context) error {
		// Hạ bản hiện hành trước để không vi phạm index một bản ACTIVE.
		var active struct {
			Version int `bson:"version"`
		}
		err := collection.FindOneAndUpdate(sc, bson.M{"package_code": t.Code, "status": "ACTIVE"},
			bson.M{"$set": bson.M{"status": "SUPERSEDED"}}).Decode(&active)
		if err != nil && !errors.Is(err, mongodrv.ErrNoDocuments) {
			return err
		}
		update := bson.M{"status": "ACTIVE", "published_by": t.ActorID, "published_at": t.At}
		if t.Kind == domain.KindSavings {
			update["effective_at"] = t.At
		}
		result, err := collection.UpdateOne(sc,
			bson.M{"package_code": t.Code, "version": t.ExpectedVersion, "status": "DRAFT"}, bson.M{"$set": update})
		if err != nil {
			return err
		}
		if result.MatchedCount == 0 {
			return appcatalog.ErrVersionConflict
		}
		return s.writeAudit(sc, id, t, "publish", active.Version, t.ExpectedVersion)
	})
	if err != nil {
		return domain.Package{}, err
	}
	items, err := s.find(ctx, t.Kind, bson.M{"package_code": t.Code, "version": t.ExpectedVersion})
	if err != nil || len(items) != 1 {
		return domain.Package{}, errors.Join(err, errors.New("published package missing"))
	}
	return items[0], nil
}

func (s *ProductCatalogStore) Retire(ctx context.Context, t appcatalog.Transition) (domain.Package, error) {
	id := auditID(t, "retire")
	collection := s.db.Collection(catalogCollectionName(t.Kind))
	if p, found, err := s.replayed(ctx, id, t.Kind, t.Code); err != nil || found {
		return p, err
	}
	err := s.transaction(ctx, func(sc context.Context) error {
		result, err := collection.UpdateOne(sc,
			bson.M{"package_code": t.Code, "version": t.ExpectedVersion, "status": "ACTIVE"},
			bson.M{"$set": bson.M{"status": "RETIRED", "retired_by": t.ActorID, "retired_at": t.At, "retire_reason": t.Reason}})
		if err != nil {
			return err
		}
		if result.MatchedCount == 0 {
			return appcatalog.ErrVersionConflict
		}
		return s.writeAudit(sc, id, t, "retire", t.ExpectedVersion, t.ExpectedVersion)
	})
	if err != nil {
		return domain.Package{}, err
	}
	items, err := s.find(ctx, t.Kind, bson.M{"package_code": t.Code, "version": t.ExpectedVersion})
	if err != nil || len(items) != 1 {
		return domain.Package{}, errors.Join(err, errors.New("retired package missing"))
	}
	return items[0], nil
}

func (s *ProductCatalogStore) Usage(ctx context.Context, kind domain.Kind, code string) (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	collection, match := "loan_applications_read", bson.M{"application.loanpackageid": code, "status": bson.M{"$ne": "deleted"}}
	if kind == domain.KindSavings {
		collection, match = "savings_contracts", bson.M{"package_id": code}
	}
	cur, err := s.db.Collection(collection).Aggregate(ctx, mongodrv.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{"_id": "$status", "count": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	usage := map[string]int64{}
	for cur.Next(ctx) {
		var row struct {
			Status string `bson:"_id"`
			Count  int64  `bson:"count"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		usage[row.Status] = row.Count
	}
	return usage, cur.Err()
}

func (s *ProductCatalogStore) Audit(ctx context.Context, kind domain.Kind, code string, limit int) ([]appcatalog.AuditEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	cur, err := s.db.Collection(catalogAuditCollection).Find(ctx, bson.M{"kind": string(kind), "code": code},
		options.Find().SetSort(bson.D{{Key: "at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var rows []struct {
		Action  string    `bson:"action"`
		Version string    `bson:"version"`
		ActorID string    `bson:"actor_id"`
		At      time.Time `bson:"at"`
		Reason  string    `bson:"reason"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	items := make([]appcatalog.AuditEntry, 0, len(rows))
	for _, row := range rows {
		items = append(items, appcatalog.AuditEntry{Action: row.Action, Version: row.Version, ActorID: row.ActorID, At: row.At, Reason: row.Reason})
	}
	return items, nil
}

type termsRow struct {
	Version      string                 `bson:"version"`
	Status       string                 `bson:"status"`
	Title        string                 `bson:"title"`
	Introduction string                 `bson:"introduction"`
	Sections     []savings.TermsSection `bson:"sections"`
	CreatedAt    time.Time              `bson:"created_at"`
}

func (r termsRow) toDocument() appcatalog.TermsDocument {
	return appcatalog.TermsDocument{Version: r.Version, Status: r.Status, Title: r.Title, Introduction: r.Introduction, Sections: r.Sections}
}

func (s *ProductCatalogStore) ListTerms(ctx context.Context) ([]appcatalog.TermsDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	cur, err := s.db.Collection("savings_terms").Find(ctx, bson.M{"product_code": savingsTermsProduct})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var rows []termsRow
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	// Bản nháp lên đầu, rồi bản mới nhất; bản seed không có created_at.
	sort.SliceStable(rows, func(i, j int) bool {
		if (rows[i].Status == "DRAFT") != (rows[j].Status == "DRAFT") {
			return rows[i].Status == "DRAFT"
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	items := make([]appcatalog.TermsDocument, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDocument())
	}
	return items, nil
}

// SaveTermsDraft tạo hoặc cập nhật bản nháp duy nhất. Mã phiên bản mới được sinh
// theo thời gian, vì khách đồng ý theo mã này và mã cũ không được dùng lại.
func (s *ProductCatalogStore) SaveTermsDraft(ctx context.Context, in appcatalog.TermsDocument, actorID string, at time.Time) (appcatalog.TermsDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	collection := s.db.Collection("savings_terms")
	version := "savings-standard-" + at.Format("20060102T150405Z")
	var existing termsRow
	err := collection.FindOne(ctx, bson.M{"product_code": savingsTermsProduct, "status": "DRAFT"}).Decode(&existing)
	switch {
	case err == nil:
		version = existing.Version
	case !errors.Is(err, mongodrv.ErrNoDocuments):
		return appcatalog.TermsDocument{}, err
	}
	_, err = collection.UpdateOne(ctx, bson.M{"_id": version, "status": "DRAFT"}, bson.M{
		"$set":         bson.M{"title": in.Title, "introduction": in.Introduction, "sections": in.Sections, "updated_by": actorID},
		"$setOnInsert": bson.M{"product_code": savingsTermsProduct, "status": "DRAFT", "version": version, "created_at": at},
	}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return appcatalog.TermsDocument{}, err
	}
	in.Version, in.Status = version, "DRAFT"
	return in, nil
}

func (s *ProductCatalogStore) DeleteTermsDraft(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	result, err := s.db.Collection("savings_terms").DeleteOne(ctx, bson.M{"product_code": savingsTermsProduct, "status": "DRAFT"})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return appcatalog.ErrTermsNotFound
	}
	return nil
}

func (s *ProductCatalogStore) PublishTerms(ctx context.Context, version, actorID string, at time.Time) (appcatalog.TermsDocument, error) {
	collection := s.db.Collection("savings_terms")
	var published termsRow
	err := s.transaction(ctx, func(sc context.Context) error {
		if err := collection.FindOne(sc, bson.M{"_id": version, "product_code": savingsTermsProduct, "status": "DRAFT"}).Decode(&published); err != nil {
			if errors.Is(err, mongodrv.ErrNoDocuments) {
				return appcatalog.ErrVersionConflict
			}
			return err
		}
		if _, err := collection.UpdateMany(sc, bson.M{"product_code": savingsTermsProduct, "status": "ACTIVE"}, bson.M{"$set": bson.M{"status": "INACTIVE"}}); err != nil {
			return err
		}
		if _, err := collection.UpdateOne(sc, bson.M{"_id": version}, bson.M{"$set": bson.M{"status": "ACTIVE", "published_by": actorID, "published_at": at}}); err != nil {
			return err
		}
		_, err := s.db.Collection(catalogAuditCollection).InsertOne(sc, bson.M{
			"kind": "savings_terms", "code": savingsTermsProduct, "action": "publish", "version": version, "actor_id": actorID, "at": at,
		})
		return err
	})
	if err != nil {
		return appcatalog.TermsDocument{}, err
	}
	published.Status = "ACTIVE"
	return published.toDocument(), nil
}
