//go:build e2e

package eventstore_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"golang.org/x/crypto/bcrypt"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
)

// A verified account proves profile saves work with real identity data, and a
// fresh repository proves the changes survive replay of the event stream.
func TestProfileEventSourcing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	mc, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, mc)
	uri, err := mc.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsedURI, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	query := parsedURI.Query()
	query.Set("directConnection", "true")
	parsedURI.RawQuery = query.Encode()
	client, err := mongorepo.NewMongoClient(ctx, &domain.MongoConfig{MongoURI: parsedURI.String()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := client.Database("profile_event_sourcing")
	applyMigrations(t, ctx, db)
	es := startEventStore(t, ctx)
	transfers := mongorepo.NewTransferRepository(client, db.Name())
	bank := eventstore.NewBankRepository(es, transfers)

	registered, err := commands.NewRegisterAccountCommandHandler(bank, bank).Handle(ctx, commands.RegisterAccountCommand{IdempotencyKey: uuid.NewString(), Username: "profile-user", Email: "profile@example.com", Password: "OldPassw0rd!", CCCDNumber: "000000000001", CCCDIssuedDate: time.Now().Add(-time.Hour), DOB: time.Now().AddDate(-25, 0, 0)})
	if err != nil {
		t.Fatal(err)
	}
	id := registered.Id
	if _, err := commands.NewVerifyAccountCommandHandler(bank).Handle(ctx, commands.VerifyAccountCommand{AccountID: id, IdCardFrontUrl: "kyc/" + id + "/id-card-front/front", IdCardBackUrl: "kyc/" + id + "/id-card-back/back", LiveVideoUrl: "kyc/" + id + "/live-video/live", Evidence: &accountdomain.KYCEvidence{MatchingStatus: "passed", LivenessStatus: "passed", ChallengePassed: true, SubjectConsistencyPassed: true, ModelVersion: "e2e-fixture"}, MatchScore: 0.9, LivenessScore: 1}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	stale, err := bank.FindByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.NewUpdateCustomerProfileHandler(bank).Handle(ctx, commands.UpdateCustomerProfileCommand{AccountID: id, Phone: "+84901234567"}); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	// A snapshot read before the update must conflict rather than overwrite it.
	stale.Customer.Profile.Phone = "+84900000000"
	stale.Version++
	if err := bank.Save(ctx, stale); !errors.Is(err, accountdomain.ErrConcurrentUpdate) {
		t.Fatalf("stale save error = %v", err)
	}

	if err := commands.NewChangePasswordHandler(bank).Handle(ctx, commands.ChangePasswordCommand{AccountID: id, CurrentPassword: "OldPassw0rd!", NewPassword: "NewPassw0rd!"}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	// Saving another profile change afterwards must not restore the old credential.
	phone := "+84907654321"
	if _, err := commands.NewUpdateCustomerProfileHandler(bank).Handle(ctx, commands.UpdateCustomerProfileCommand{AccountID: id, Phone: phone}); err != nil {
		t.Fatalf("update after password change: %v", err)
	}

	replayed := eventstore.NewBankRepository(es, transfers)
	account, err := replayed.FindByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte("NewPassw0rd!")); err != nil {
		t.Fatalf("replay lost new password: %v", err)
	}
	version, err := replayed.TokenVersion(ctx, id)
	if err != nil || version != 1 {
		t.Fatalf("token version = %d/%v", version, err)
	}
	if account.Customer.Profile.Phone != phone {
		t.Fatalf("replayed phone = %q", account.Customer.Profile.Phone)
	}
	if !account.IsVerified() {
		t.Fatal("verification lost after profile updates")
	}
}
