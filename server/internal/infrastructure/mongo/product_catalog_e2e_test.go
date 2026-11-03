//go:build e2e

package mongo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appcatalog "github.com/dont-wait/anomaly/internal/application/catalog"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	domain "github.com/dont-wait/anomaly/internal/domain/catalog"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// catalogTestMongo dùng replica set thật vì Publish cần transaction.
func catalogTestMongo(t *testing.T) (*mongodrv.Client, string) {
	t.Helper()
	ctx := context.Background()
	container, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, container)
	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mongodrv.Connect(options.Client().ApplyURI(uri).SetDirect(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(ctx) })
	return client, "catalog_" + uuid.NewString()[:8]
}

func auditCount(t *testing.T, store *ProductCatalogStore, code string) int64 {
	t.Helper()
	count, err := store.db.Collection(catalogAuditCollection).CountDocuments(context.Background(), bson.M{"code": code})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestProductCatalogSavingsLifecycle(t *testing.T) {
	client, dbName := catalogTestMongo(t)
	ctx := context.Background()
	reader := NewSavingsPackageCatalog(client, dbName)
	if err := reader.EnsureSeeded(ctx); err != nil {
		t.Fatal(err)
	}
	// Chạy lại phải an toàn, giống mỗi lần API khởi động.
	if err := reader.EnsureSeeded(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewProductCatalogStore(client, dbName)
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	service := &appcatalog.Service{Store: store}
	code := savingsqueries.StandardPackageID

	draft, err := service.CreateDraft(ctx, domain.KindSavings, code, "admin")
	if err != nil || draft.Version != 2 {
		t.Fatalf("draft = %+v, err = %v", draft, err)
	}
	if _, err := service.CreateDraft(ctx, domain.KindSavings, code, "admin"); !errors.Is(err, appcatalog.ErrDraftExists) {
		t.Fatalf("err = %v", err)
	}
	_, err = service.UpdateDraft(ctx, domain.KindSavings, code, appcatalog.DraftInput{
		Name: "Tiết kiệm mới", MinAmount: 2_000_000,
		Terms: []domain.TermOption{{Months: 12, AnnualRateBps: 625}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Bản đang bán không đổi cho đến khi phát hành.
	before, err := reader.ListActive(ctx)
	if err != nil || before[0].Version != 1 || len(before[0].TermOptions) != 7 {
		t.Fatalf("before = %+v, err = %v", before, err)
	}

	key := uuid.NewString()
	if _, err := service.Publish(ctx, domain.KindSavings, code, "admin", key, 2); err != nil {
		t.Fatal(err)
	}
	after, err := reader.ListActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Version != 2 || after[0].MinimumDeposit != 2_000_000 || after[0].TermOptions[0].AnnualInterestRate != "6.25" {
		t.Fatalf("after = %+v", after[0])
	}

	// Retry cùng khóa không tạo thêm audit hay phiên bản.
	audits := auditCount(t, store, code)
	if _, err := service.Publish(ctx, domain.KindSavings, code, "admin", key, 2); err != nil {
		t.Fatal(err)
	}
	if got := auditCount(t, store, code); got != audits || audits != 1 {
		t.Fatalf("audits = %d then %d", audits, got)
	}

	// Khóa khác trên bản đã phát hành là xung đột.
	if _, err := service.Publish(ctx, domain.KindSavings, code, "admin", uuid.NewString(), 2); !errors.Is(err, appcatalog.ErrVersionConflict) {
		t.Fatalf("err = %v", err)
	}

	if _, err := service.Retire(ctx, domain.KindSavings, code, "admin", uuid.NewString(), "Ngừng bán", 1); !errors.Is(err, appcatalog.ErrVersionConflict) {
		t.Fatalf("stale retire err = %v", err)
	}
	if _, err := service.Retire(ctx, domain.KindSavings, code, "admin", uuid.NewString(), "Ngừng bán", 2); err != nil {
		t.Fatal(err)
	}
	retired, err := reader.ListActive(ctx)
	if err != nil || len(retired) != 0 {
		t.Fatalf("retired = %+v, err = %v", retired, err)
	}
	// Khởi động lại khi đang ngừng bán không được báo lỗi hay seed lại.
	if err := reader.EnsureSeeded(ctx); err != nil {
		t.Fatalf("restart with retired package must not disable savings: %v", err)
	}
	if again, _ := reader.ListActive(ctx); len(again) != 0 {
		t.Fatalf("retired package was seeded again: %+v", again)
	}
}

func TestProductCatalogConcurrentPublishHasOneWinner(t *testing.T) {
	client, dbName := catalogTestMongo(t)
	ctx := context.Background()
	reader := NewLoanPackageCatalog(client, dbName)
	if err := reader.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.SeedDefaultPackage(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewProductCatalogStore(client, dbName)
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	service := &appcatalog.Service{Store: store}
	const code = "PERSONAL_UNSECURED_SALARY"
	if _, err := service.CreateDraft(ctx, domain.KindLoan, code, "admin"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = service.Publish(ctx, domain.KindLoan, code, "admin", uuid.NewString(), 2)
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("publish results = %v", results)
	}
	versions, err := service.Get(ctx, domain.KindLoan, code)
	if err != nil || versions.ActiveVersion != 2 || len(versions.Audit) != 1 {
		t.Fatalf("detail = %+v, err = %v", versions, err)
	}
	items, err := reader.ListActiveLoanPackages(ctx)
	if err != nil || len(items) != 1 || items[0].Version != 2 {
		t.Fatalf("items = %+v, err = %v", items, err)
	}
}

func TestProductCatalogFailedPublishLeavesNoChange(t *testing.T) {
	client, dbName := catalogTestMongo(t)
	ctx := context.Background()
	reader := NewLoanPackageCatalog(client, dbName)
	if err := reader.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.SeedDefaultPackage(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewProductCatalogStore(client, dbName)
	const code = "PERSONAL_UNSECURED_SALARY"
	// Không có bản nháp v9: bản ACTIVE đã hạ trong transaction phải được khôi phục.
	_, err := store.Publish(ctx, appcatalog.Transition{Kind: domain.KindLoan, Code: code, ExpectedVersion: 9, ActorID: "a", IdempotencyKey: uuid.NewString(), At: time.Now().UTC()})
	if !errors.Is(err, appcatalog.ErrVersionConflict) {
		t.Fatalf("err = %v", err)
	}
	items, err := reader.ListActiveLoanPackages(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("active package lost: %+v, %v", items, err)
	}
	if got := auditCount(t, store, code); got != 0 {
		t.Fatalf("audits = %d", got)
	}
}

func TestProductCatalogTerms(t *testing.T) {
	client, dbName := catalogTestMongo(t)
	ctx := context.Background()
	store := NewProductCatalogStore(client, dbName)
	service := &appcatalog.Service{Store: store}
	if _, err := service.SaveTermsDraft(ctx, appcatalog.TermsDocument{Title: "x"}, "admin"); !errors.Is(err, domain.ErrInvalidPackage) {
		t.Fatalf("err = %v", err)
	}
	draft := appcatalog.TermsDocument{Title: "Điều kiện", Introduction: "Giới thiệu"}
	draft.Sections = []savings.TermsSection{{Title: "Mục", Body: "Nội dung"}}
	saved, err := service.SaveTermsDraft(ctx, draft, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishTerms(ctx, "khac", "admin"); !errors.Is(err, appcatalog.ErrVersionConflict) {
		t.Fatalf("err = %v", err)
	}
	if _, err := service.PublishTerms(ctx, saved.Version, "admin"); err != nil {
		t.Fatal(err)
	}
	current, err := NewSavingsTermsRepository(client, dbName).Current(ctx)
	if err != nil || current.Version != saved.Version {
		t.Fatalf("current = %+v, err = %v", current, err)
	}
}

func TestProductCatalogCreateSellDeleteNewPackages(t *testing.T) {
	client, dbName := catalogTestMongo(t)
	ctx := context.Background()
	reader := NewSavingsPackageCatalog(client, dbName)
	if err := reader.EnsureSeeded(ctx); err != nil {
		t.Fatal(err)
	}
	loans := NewLoanPackageCatalog(client, dbName)
	if err := loans.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewProductCatalogStore(client, dbName)
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	service := &appcatalog.Service{Store: store}

	// Gói tiết kiệm mới bán song song với gói mặc định.
	savingsIn := appcatalog.CreateInput{Code: "tiet_kiem_vip", DraftInput: appcatalog.DraftInput{
		Name: "Tiết kiệm VIP", MinAmount: 5_000_000, Terms: []domain.TermOption{{Months: 12, AnnualRateBps: 650}},
	}}
	created, err := service.CreatePackage(ctx, domain.KindSavings, savingsIn, "admin")
	if err != nil || created.Code != "TIET_KIEM_VIP" {
		t.Fatalf("created = %+v, err = %v", created, err)
	}
	if _, err := service.CreatePackage(ctx, domain.KindSavings, savingsIn, "admin"); !errors.Is(err, appcatalog.ErrCodeExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	if items, err := reader.ListActive(ctx); err != nil || len(items) != 1 {
		t.Fatalf("draft must not be sold: %+v, err = %v", items, err)
	}
	if _, err := service.Publish(ctx, domain.KindSavings, created.Code, "admin", uuid.NewString(), 1); err != nil {
		t.Fatal(err)
	}
	items, err := reader.ListActive(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("both packages must be on sale: %+v, err = %v", items, err)
	}
	if err := service.DeletePackage(ctx, domain.KindSavings, created.Code, "admin"); !errors.Is(err, appcatalog.ErrPackageActive) {
		t.Fatalf("active delete err = %v", err)
	}
	if _, err := service.Retire(ctx, domain.KindSavings, created.Code, "admin", uuid.NewString(), "hết chương trình", 1); err != nil {
		t.Fatal(err)
	}
	if err := service.DeletePackage(ctx, domain.KindSavings, created.Code, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, domain.KindSavings, created.Code); !errors.Is(err, appcatalog.ErrPackageNotFound) {
		t.Fatalf("deleted package still readable: %v", err)
	}
	if _, err := service.CreatePackage(ctx, domain.KindSavings, savingsIn, "admin"); !errors.Is(err, appcatalog.ErrCodeExists) {
		t.Fatalf("deleted code must not be reused: %v", err)
	}

	// Gói đã có hợp đồng không xóa được.
	if _, err := service.Retire(ctx, domain.KindSavings, savingsqueries.StandardPackageID, "admin", uuid.NewString(), "x", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Database(dbName).Collection("savings_contracts").InsertOne(ctx, bson.M{"package_id": savingsqueries.StandardPackageID, "status": "settled"}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeletePackage(ctx, domain.KindSavings, savingsqueries.StandardPackageID, "admin"); !errors.Is(err, appcatalog.ErrPackageInUse) {
		t.Fatalf("used delete err = %v", err)
	}

	// Gói vay mới dùng được ngay khi tạo hồ sơ; bản nháp thì chưa.
	loanIn := appcatalog.CreateInput{Code: "VAY_MUA_XE", DraftInput: appcatalog.DraftInput{
		Name: "Vay mua xe", MinAmount: 10_000_000, MaxAmount: 900_000_000, Terms: []domain.TermOption{{Months: 24}},
	}}
	if _, err := service.CreatePackage(ctx, domain.KindLoan, loanIn, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := loans.ValidateLoanPackage(ctx, "VAY_MUA_XE", 50_000_000, 24); err == nil {
		t.Fatal("draft loan package must not be accepted")
	}
	if _, err := service.Publish(ctx, domain.KindLoan, "VAY_MUA_XE", "admin", uuid.NewString(), 1); err != nil {
		t.Fatal(err)
	}
	if version, err := loans.ValidateLoanPackage(ctx, "VAY_MUA_XE", 50_000_000, 24); err != nil || version != 1 {
		t.Fatalf("version = %d, err = %v", version, err)
	}
	if _, err := loans.ValidateLoanPackage(ctx, "VAY_MUA_XE", 50_000_000, 12); err == nil {
		t.Fatal("term outside the new package must be rejected")
	}

	// Xóa bản nháp chưa từng phát hành thì giải phóng luôn mã.
	if _, err := service.CreatePackage(ctx, domain.KindLoan, appcatalog.CreateInput{Code: "NHAP", DraftInput: loanIn.DraftInput}, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := service.DeletePackage(ctx, domain.KindLoan, "NHAP", "admin"); err != nil {
		t.Fatal(err)
	}
	if listed, err := service.List(ctx, domain.KindLoan, "", "NHAP"); err != nil || len(listed) != 0 {
		t.Fatalf("listed = %+v, err = %v", listed, err)
	}
}
