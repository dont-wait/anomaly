//go:build e2e

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
)

func TestMigrationsAgainstMongoDB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mongoContainer, err := mongodb.Run(ctx, "mongo:7.0")
	if err != nil {
		t.Fatalf("start MongoDB container: %v", err)
	}
	testcontainers.CleanupContainer(t, mongoContainer)

	mongoURI, err := mongoContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("get MongoDB connection string: %v", err)
	}

	const databaseName = "anomaly_migration_e2e"
	t.Setenv("MONGO_URI", mongoURI)
	t.Setenv("MONGO_DB", databaseName)
	databaseURL, err := mongoDatabaseURL(domain.GetEnvLoader())
	if err != nil {
		t.Fatalf("build migration database URL: %v", err)
	}

	runner, err := migrate.New("file://../../migrations", databaseURL)
	if err != nil {
		t.Fatalf("create migration runner: %v", err)
	}
	t.Cleanup(func() {
		_, _ = runner.Close()
	})

	if err := runner.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	version, dirty, err := runner.Version()
	if err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != 3 || dirty {
		t.Fatalf("migration state = version %d dirty %t, want version 3 dirty false", version, dirty)
	}

	mongoConfig := &domain.MongoConfig{MongoURI: mongoURI, MongoDBName: databaseName}
	client, err := mongorepo.NewMongoClient(ctx, mongoConfig)
	if err != nil {
		t.Fatalf("connect to migrated MongoDB: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.Disconnect(cleanupCtx)
	})

	database := client.Database(databaseName)
	for _, collectionName := range []string{"customers", "kyc_sessions", "accounts"} {
		_, err := database.Collection(collectionName).InsertOne(ctx, bson.D{{Key: "invalid", Value: true}})
		if err == nil {
			t.Fatalf("insert invalid document into %s succeeded; validator was not enforced", collectionName)
		}
	}

	repository := mongorepo.NewAccountAggregateRepository(client, databaseName)
	register := commands.NewRegisterAccountCommandHandler(repository, repository)
	account, err := register.Handle(ctx, commands.RegisterAccountCommand{
		Username: "migration-e2e",
		Email:    fmt.Sprintf("migration-%d@example.com", time.Now().UnixNano()),
		Password: "migration-e2e-password",
	})
	if err != nil {
		t.Fatalf("register account against migrated schema: %v", err)
	}

	verify := commands.NewVerifyAccountCommandHandler(repository)
	if _, err := verify.Handle(ctx, commands.VerifyAccountCommand{
		AccountID:      account.Id,
		IdCardFrontUrl: "media/migration-front.jpg",
		IdCardBackUrl:  "media/migration-back.jpg",
		LiveVideoUrl:   "media/migration-live.mp4",
	}); err != nil {
		t.Fatalf("verify account against migrated schema: %v", err)
	}

	if err := runner.Steps(-3); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if _, _, err := runner.Version(); !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatalf("migration version after down = %v, want ErrNilVersion", err)
	}

	for _, collectionName := range []string{"customers", "kyc_sessions", "accounts"} {
		if _, err := database.Collection(collectionName).InsertOne(ctx, bson.D{{Key: "invalid_after_down", Value: true}}); err != nil {
			t.Fatalf("insert into %s after validator rollback: %v", collectionName, err)
		}
	}
}
