//go:build e2e

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
)

func TestConcurrentRegistrationPersistsOneAccountWithoutEventStore(t *testing.T) {
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

	mongoConfig := &domain.MongoConfig{
		MongoURI:    mongoURI,
		MongoDBName: "anomaly_account_e2e",
	}
	mongoClient, err := mongorepo.NewMongoClient(ctx, mongoConfig)
	if err != nil {
		t.Fatalf("connect to MongoDB container: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = mongoClient.Disconnect(cleanupCtx)
	}()

	repo := mongorepo.NewAccountAggregateRepository(mongoClient, mongoConfig.MongoDBName)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("ensure MongoDB indexes: %v", err)
	}
	register := commands.NewRegisterAccountCommandHandler(repo, repo)

	const requestCount = 8
	sharedEmail := fmt.Sprintf("concurrent-%d@example.com", time.Now().UnixNano())
	start := make(chan struct{})
	results := make(chan error, requestCount)
	accounts := make(chan string, requestCount)
	for index := range requestCount {
		go func() {
			<-start
			account, handleErr := register.Handle(ctx, commands.RegisterAccountCommand{
				Username: fmt.Sprintf("concurrent-user-%d", index),
				Email:    sharedEmail,
				Password: "e2e-password-123",
			})
			if account != nil {
				accounts <- account.Id
			}
			results <- handleErr
		}()
	}
	close(start)

	successes := 0
	conflicts := 0
	for range requestCount {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, accountdomain.ErrUserAlreadyExists):
			conflicts++
		default:
			t.Fatalf("register account: %v", err)
		}
	}
	if successes != 1 || conflicts != requestCount-1 {
		t.Fatalf("registration results = %d success, %d conflicts; want 1 and %d", successes, conflicts, requestCount-1)
	}

	close(accounts)
	accountID := <-accounts
	verify := commands.NewVerifyAccountCommandHandler(repo)
	if _, err := verify.Handle(ctx, commands.VerifyAccountCommand{
		AccountID:      accountID,
		IdCardFrontUrl: "media/e2e-front.jpg",
		IdCardBackUrl:  "media/e2e-back.jpg",
		LiveVideoUrl:   "media/e2e-live.mp4",
	}); err != nil {
		t.Fatalf("verify MongoDB account: %v", err)
	}

	stored, err := repo.FindByID(ctx, accountID)
	if err != nil {
		t.Fatalf("load MongoDB account: %v", err)
	}
	if stored == nil || stored.Customer == nil {
		t.Fatal("stored account aggregate is incomplete")
	}
	if len(stored.KYCSessions) != 1 || !stored.IsVerified() {
		t.Fatalf("stored verification state = sessions %d, verified %v; want 1 and true", len(stored.KYCSessions), stored.IsVerified())
	}
	second, err := register.Handle(ctx, commands.RegisterAccountCommand{
		Username: "second-account",
		Email:    fmt.Sprintf("second-%d@example.com", time.Now().UnixNano()),
		Password: "e2e-password-123",
	})
	if err != nil {
		t.Fatalf("register second account: %v", err)
	}

	originalEmail := stored.Email
	originalName := stored.Customer.Profile.FullName
	stored.Email = second.Email
	stored.Customer.Profile.FullName = "must-be-rolled-back"
	now := time.Now().UTC()
	stored.KYCSessions = append(stored.KYCSessions, &accountdomain.KYCSession{
		Id:         bson.NewObjectID().Hex(),
		CustomerId: stored.CustomerId,
		AttemptNo:  2,
		Status:     accountdomain.KYCSessionStatusVerified,
		Media: accountdomain.KYCMedia{
			IdentityFront: accountdomain.MediaObject{StorageKey: "media/rollback-front.jpg"},
			IdentityBack:  accountdomain.MediaObject{StorageKey: "media/rollback-back.jpg"},
			LivenessVideo: accountdomain.LivenessVideo{
				MediaObject: accountdomain.MediaObject{StorageKey: "media/rollback-live.mp4"},
			},
		},
		StartedAt:   now,
		CompletedAt: &now,
		CreatedAt:   now,
	})
	if err := repo.Save(ctx, stored); !errors.Is(err, accountdomain.ErrUserAlreadyExists) {
		t.Fatalf("save conflicting aggregate error = %v, want ErrUserAlreadyExists", err)
	}

	restored, err := repo.FindByID(ctx, accountID)
	if err != nil {
		t.Fatalf("load compensated aggregate: %v", err)
	}
	if restored.Email != originalEmail || restored.Customer.Profile.FullName != originalName {
		t.Fatalf("compensated account/customer = %q/%q, want %q/%q", restored.Email, restored.Customer.Profile.FullName, originalEmail, originalName)
	}
	if len(restored.KYCSessions) != 1 {
		t.Fatalf("compensated KYC session count = %d, want 1", len(restored.KYCSessions))
	}

	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("list MongoDB accounts: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("MongoDB account count = %d, want 2", len(all))
	}
}
