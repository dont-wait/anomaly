// Command seed-anomaly-data imports the generated A-F fixture into the
// canonical banking stream. It is intentionally separate from the normal API
// and is guarded as a development-only command.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
)

const testPassword = "Password123@"

type dataset struct {
	SchemaVersion int              `json:"schema_version"`
	DatasetID     string           `json:"dataset_id"`
	Accounts      []fixtureAccount `json:"accounts"`
}

type fixtureAccount struct {
	ID                          string          `json:"id"`
	Alias                       string          `json:"alias"`
	Kind                        string          `json:"kind"`
	Case                        string          `json:"case"`
	Username                    string          `json:"username"`
	FullName                    string          `json:"full_name"`
	CCCD                        string          `json:"cccd"`
	Email                       string          `json:"email"`
	Password                    string          `json:"password"`
	AccountNo                   string          `json:"account_no"`
	KnownDeviceID               string          `json:"known_device_id"`
	StartingBalance             int64           `json:"starting_balance"`
	ExpectedBalanceAfterHistory int64           `json:"expected_balance_after_history"`
	History                     json.RawMessage `json:"history"`
}

type fixtureTransfer struct {
	ID                 string `json:"id"`
	IdempotencyKey     string `json:"idempotency_key"`
	DestinationAccount string `json:"destination_account"`
	Amount             int64  `json:"amount"`
	Currency           string `json:"currency"`
	BankCode           string `json:"bank_code"`
	Note               string `json:"note"`
	DeviceID           string `json:"device_id"`
	Platform           string `json:"platform"`
	OccurredAt         string `json:"occurred_at"`
	CompletedAt        string `json:"completed_at"`
}

type importedAccount struct {
	FixtureID    string `json:"fixture_id"`
	Alias        string `json:"alias"`
	Kind         string `json:"kind"`
	Case         string `json:"case"`
	PublicID     string `json:"public_id"`
	AccountNo    string `json:"account_no"`
	CCCD         string `json:"cccd"`
	Password     string `json:"password"`
	HistoryCount int    `json:"history_count"`
}

type importManifest struct {
	SchemaVersion string            `json:"schema_version"`
	DatasetID     string            `json:"dataset_id"`
	Status        string            `json:"status"`
	ImportedAt    string            `json:"imported_at"`
	Accounts      []importedAccount `json:"accounts"`
}

func main() {
	log := logger.NewLogger(zerolog.InfoLevel)
	file := flag.String("file", "../tools/test-data/output/anomaly-a-f.json", "generated anomaly fixture JSON")
	manifest := flag.String("manifest", "", "optional output path for actual account numbers and credentials")
	waitForReady := flag.Duration("wait-for-ready", 0, "wait until banking projection and model feature state catch up")
	flag.Parse()

	data, err := readDataset(*file)
	if err != nil {
		log.Fatal().Err(err).Msg("read anomaly fixture failed")
	}
	if err := validateDataset(data); err != nil {
		log.Fatal().Err(err).Msg("anomaly fixture validation failed")
	}

	loader := domain.GetEnvLoader().Load(log)
	config := loader.LoadEventStoreConfig()
	if err := validateEnvironment(config.EventStoreConnString); err != nil {
		log.Fatal().Err(err).Msg("anomaly fixture import refused")
	}
	client, err := eventstore.NewEventStoreClient(config)
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore failed")
	}
	defer eventstore.Disconnect(client)

	repo := eventstore.SeedRepository{BankRepository: eventstore.NewBankRepository(client, nil)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	manifestPath := *manifest
	if manifestPath == "" {
		manifestPath = filepath.Join(filepath.Dir(*file), "anomaly-a-f.imported.json")
	}
	if err := writeManifest(manifestPath, data.DatasetID, "in_progress", nil); err != nil {
		log.Fatal().Err(err).Msg("write in-progress anomaly manifest failed")
	}

	accounts, err := importDataset(ctx, &repo, data)
	if err != nil {
		log.Fatal().Err(err).Msg("import anomaly fixture failed")
	}

	targetRevision, err := repo.CurrentRevision(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("read imported EventStoreDB revision failed")
	}
	if *waitForReady > 0 {
		readyCtx, readyCancel := context.WithTimeout(context.Background(), *waitForReady)
		defer readyCancel()
		if err := waitForReadiness(readyCtx, loader.LoadMongoConfig(), targetRevision); err != nil {
			log.Fatal().Err(err).Msg("anomaly fixture readiness check failed")
		}
	}
	if err := writeManifest(manifestPath, data.DatasetID, "complete", accounts); err != nil {
		log.Fatal().Err(err).Msg("write anomaly fixture manifest failed")
	}

	fmt.Printf("Imported dataset %s\n", data.DatasetID)
	for _, account := range accounts {
		fmt.Printf("%s\t%s\tCCCD=%s\tpassword=%s\taccount_no=%s\thistory=%d\n", account.Alias, account.Case, account.CCCD, account.Password, account.AccountNo, account.HistoryCount)
	}
	fmt.Printf("Manifest: %s\n", manifestPath)
}

func validateEnvironment(eventStoreConnString string) error {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv != "development" && appEnv != "dev" && appEnv != "local" && appEnv != "test" {
		return fmt.Errorf("APP_ENV must be development, dev, local, or test; got %q", appEnv)
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("SEED_DEMO_ENABLED")), "true") {
		return errors.New("SEED_DEMO_ENABLED=true is required")
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("SEED_ANOMALY_ALLOW_TARGET")), "true") {
		return errors.New("SEED_ANOMALY_ALLOW_TARGET=true is required")
	}
	parsed, err := url.Parse(eventStoreConnString)
	if err != nil || (parsed.Scheme != "esdb" && parsed.Scheme != "esdb+discover") {
		return errors.New("anomaly fixture requires an esdb development connection string")
	}
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "::1", "eventstore", "kurrentdb":
	default:
		return fmt.Errorf("refusing non-local EventStoreDB target %q", host)
	}
	if parsed.User != nil {
		return errors.New("refusing EventStoreDB connection strings with credentials")
	}
	if strings.EqualFold(parsed.Query().Get("tls"), "true") {
		return errors.New("refusing TLS EventStoreDB targets; fixture import is local-only")
	}
	return nil
}

func readDataset(path string) (dataset, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return dataset{}, err
	}
	var result dataset
	if err := json.Unmarshal(content, &result); err != nil {
		return dataset{}, err
	}
	return result, nil
}

func validateDataset(data dataset) error {
	if data.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d", data.SchemaVersion)
	}
	if strings.TrimSpace(data.DatasetID) == "" || len(data.Accounts) == 0 {
		return errors.New("dataset_id and accounts are required")
	}
	accounts := make(map[string]fixtureAccount, len(data.Accounts))
	for _, account := range data.Accounts {
		if account.ID == "" || account.CCCD == "" || accounts[account.ID].ID != "" {
			return fmt.Errorf("invalid or duplicate account id %q", account.ID)
		}
		if account.Kind != "source" && account.Kind != "receiver" {
			return fmt.Errorf("account %s has unsupported kind %q", account.ID, account.Kind)
		}
		if account.StartingBalance < 0 || account.ExpectedBalanceAfterHistory < 0 {
			return fmt.Errorf("account %s has negative balance", account.ID)
		}
		accounts[account.ID] = account
	}
	transferIDs := map[string]struct{}{}
	idempotencyKeys := map[string]struct{}{}
	for _, account := range data.Accounts {
		if account.Kind != "source" {
			continue
		}
		var history []fixtureTransfer
		if err := json.Unmarshal(account.History, &history); err != nil {
			return fmt.Errorf("decode history for %s: %w", account.ID, err)
		}
		var total int64
		for _, transfer := range history {
			if _, exists := transferIDs[transfer.ID]; exists {
				return fmt.Errorf("duplicate transfer id %s", transfer.ID)
			}
			transferIDs[transfer.ID] = struct{}{}
			if _, exists := idempotencyKeys[strings.ToLower(transfer.IdempotencyKey)]; exists {
				return fmt.Errorf("duplicate idempotency key %s", transfer.IdempotencyKey)
			}
			idempotencyKeys[strings.ToLower(transfer.IdempotencyKey)] = struct{}{}
			if _, err := uuid.Parse(transfer.ID); err != nil {
				return fmt.Errorf("transfer %s has invalid id: %w", transfer.ID, err)
			}
			if _, err := uuid.Parse(transfer.IdempotencyKey); err != nil {
				return fmt.Errorf("transfer %s has invalid idempotency key: %w", transfer.ID, err)
			}
			if transfer.Amount < 1000 || transfer.Amount > math.MaxInt64-total {
				return fmt.Errorf("transfer %s has invalid amount", transfer.ID)
			}
			if transfer.Currency != "VND" || (transfer.Platform != "mobile" && transfer.Platform != "web" && transfer.Platform != "desktop") {
				return fmt.Errorf("transfer %s has invalid currency or platform", transfer.ID)
			}
			if accounts[transfer.DestinationAccount].ID == "" || accounts[transfer.DestinationAccount].Kind != "receiver" {
				return fmt.Errorf("transfer %s references unknown receiver %q", transfer.ID, transfer.DestinationAccount)
			}
			occurredAt, err := time.Parse(time.RFC3339Nano, transfer.OccurredAt)
			if err != nil {
				return fmt.Errorf("transfer %s occurred_at: %w", transfer.ID, err)
			}
			completedAt, err := time.Parse(time.RFC3339Nano, transfer.CompletedAt)
			if err != nil || !completedAt.After(occurredAt) {
				return fmt.Errorf("transfer %s has invalid completed_at", transfer.ID)
			}
			total += transfer.Amount
		}
		if total > account.StartingBalance || account.StartingBalance-total != account.ExpectedBalanceAfterHistory {
			return fmt.Errorf("account %s history exceeds or disagrees with starting balance", account.ID)
		}
	}
	return nil
}

func importDataset(ctx context.Context, repo *eventstore.SeedRepository, data dataset) ([]importedAccount, error) {
	accountIDs := make(map[string]string, len(data.Accounts))
	imported := make([]importedAccount, 0, len(data.Accounts))

	for _, fixture := range data.Accounts {
		account, created, err := ensureFixtureAccount(ctx, repo, data.DatasetID, fixture)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", fixture.ID, err)
		}
		if fixture.ExpectedBalanceAfterHistory > 0 {
			operationID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("anomaly-balance:"+data.DatasetID+":"+fixture.ID)).String()
			if err := repo.SetFixtureBalanceOnce(ctx, account.Id, fixture.ExpectedBalanceAfterHistory, operationID); err != nil {
				return nil, fmt.Errorf("fund %s: %w", fixture.ID, err)
			}
		}
		if created {
			// The account is already present in EventStoreDB. Re-read it so the
			// manifest contains the generated public account number.
			account, err = repo.FindByID(ctx, account.Id)
			if err != nil {
				return nil, fmt.Errorf("reload account %s: %w", fixture.ID, err)
			}
		}
		accountIDs[fixture.ID] = account.Id

		historyCount := 0
		if fixture.Kind == "source" {
			var history []fixtureTransfer
			if err := json.Unmarshal(fixture.History, &history); err != nil {
				return nil, fmt.Errorf("decode history for %s: %w", fixture.ID, err)
			}
			historyCount = len(history)
		}
		imported = append(imported, importedAccount{
			FixtureID: fixture.ID, Alias: fixture.Alias, Kind: fixture.Kind, Case: fixture.Case,
			PublicID: account.Id, AccountNo: account.AccountNo, CCCD: fixture.CCCD,
			Password: effectivePassword(fixture), HistoryCount: historyCount,
		})
	}

	for _, fixture := range data.Accounts {
		if fixture.Kind != "source" {
			continue
		}
		var history []fixtureTransfer
		if err := json.Unmarshal(fixture.History, &history); err != nil {
			return nil, fmt.Errorf("decode history for %s: %w", fixture.ID, err)
		}
		seenByDestination := map[string]int64{}
		for _, transfer := range history {
			occurredAt, err := time.Parse(time.RFC3339Nano, transfer.OccurredAt)
			if err != nil {
				return nil, fmt.Errorf("transfer %s occurred_at: %w", transfer.ID, err)
			}
			completedAt, err := time.Parse(time.RFC3339Nano, transfer.CompletedAt)
			if err != nil {
				return nil, fmt.Errorf("transfer %s completed_at: %w", transfer.ID, err)
			}
			startingBalance := fixture.StartingBalance - sumHistoryBefore(history, transfer.ID)
			destinationBefore := seenByDestination[transfer.DestinationAccount]
			input := eventstore.HistoricalTransferInput{
				ID: transfer.ID, IdempotencyKey: transfer.IdempotencyKey,
				SourceAccountID: accountIDs[fixture.ID], DestinationAccountID: accountIDs[transfer.DestinationAccount],
				Amount: transfer.Amount, Currency: transfer.Currency, Channel: transfer.Platform,
				DeviceID: transfer.DeviceID, Platform: transfer.Platform, Note: transfer.Note,
				OccurredAt: occurredAt, CompletedAt: completedAt,
				SourceBalanceBefore: startingBalance, SourceBalanceAfter: startingBalance - transfer.Amount,
				DestinationBalanceBefore: destinationBefore, DestinationBalanceAfter: destinationBefore + transfer.Amount,
			}
			if err := repo.SeedHistoricalTransfer(ctx, input); err != nil {
				return nil, fmt.Errorf("transfer %s: %w", transfer.ID, err)
			}
			seenByDestination[transfer.DestinationAccount] += transfer.Amount
		}
	}
	return imported, nil
}

func ensureFixtureAccount(ctx context.Context, repo *eventstore.SeedRepository, datasetID string, fixture fixtureAccount) (*accountdomain.UserAccount, bool, error) {
	username := fixture.Username
	if username == "" {
		username = "anomaly." + strings.ReplaceAll(fixture.ID, "-", ".")
	}
	email := fixture.Email
	if email == "" {
		email = username + "@example.test"
	}
	password := effectivePassword(fixture)

	account, err := repo.FindByCCCDNumber(ctx, fixture.CCCD)
	if err != nil {
		return nil, false, err
	}
	created := false
	if account == nil {
		key := uuid.NewSHA1(uuid.NameSpaceURL, []byte("anomaly-dataset:"+datasetID+":"+fixture.ID)).String()
		handler := commands.NewRegisterAccountCommandHandler(repo, repo).AllowSeedPasswords()
		account, err = handler.Handle(ctx, commands.RegisterAccountCommand{
			IdempotencyKey: key,
			Username:       username,
			CCCDNumber:     fixture.CCCD,
			CCCDIssuedDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			DOB:            time.Date(1995, 1, 1, 0, 0, 0, 0, time.UTC),
			Email:          email,
			Password:       password,
		})
		if err != nil {
			return nil, false, err
		}
		created = true
	} else {
		if account.Username != username || account.Email != email || bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)) != nil {
			return nil, false, fmt.Errorf("existing account does not match fixture identity")
		}
	}

	changed := false
	if account.Customer.Profile.FullName != fixture.FullName && fixture.FullName != "" {
		account.Customer.Profile.FullName = fixture.FullName
		changed = true
	}
	if !account.IsVerified() {
		markFixtureAccountVerified(account)
		changed = true
	}
	if changed {
		if err := repo.Save(ctx, account); err != nil {
			return nil, created, err
		}
	}
	return account, created, nil
}

func effectivePassword(fixture fixtureAccount) string {
	if fixture.Password != "" {
		return fixture.Password
	}
	return testPassword
}

func markFixtureAccountVerified(account *accountdomain.UserAccount) {
	accountdomain.NormalizeKYCSessionIdentifiers(account)
	now := account.UpdatedAt
	if now.IsZero() {
		now = account.CreatedAt
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sessionID := accountdomain.CanonicalKYCSessionID("seed-kyc-anomaly-" + account.Id)
	completedAt := now
	account.KYCSessions = append(account.KYCSessions, &accountdomain.KYCSession{
		Id: sessionID, CustomerId: account.CustomerId, AttemptNo: len(account.KYCSessions) + 1,
		Status: accountdomain.KYCSessionStatusVerified,
		IdentityData: accountdomain.KYCIdentityData{
			Type: account.Customer.Identity.Type, Number: account.Customer.Identity.Number,
			FullName: account.Customer.Profile.FullName, DateOfBirth: account.Customer.Profile.DateOfBirth,
			IssuedDate: account.Customer.Identity.IssuedDate,
		},
		Verification: accountdomain.KYCVerification{
			OCRStatus: accountdomain.VerificationStatusNotRun, LivenessStatus: accountdomain.VerificationStatusNotRun,
			FaceMatchStatus: accountdomain.VerificationStatusNotRun, Provider: "seed",
		},
		StartedAt: now, CompletedAt: &completedAt, CreatedAt: now,
	})
	account.Customer.KYCStatus = accountdomain.KYCStatusVerified
	account.Customer.VerifiedKYCSessionId = sessionID
}

func sumHistoryBefore(history []fixtureTransfer, currentID string) int64 {
	var total int64
	for _, transfer := range history {
		if transfer.ID == currentID {
			break
		}
		total += transfer.Amount
	}
	return total
}

func waitForReadiness(ctx context.Context, config *domain.MongoConfig, targetRevision uint64) error {
	client, err := mongorepo.NewMongoClient(ctx, config)
	if err != nil {
		return fmt.Errorf("connect MongoDB: %w", err)
	}
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		mongorepo.Disconnect(client, disconnectCtx, cancel)
	}()

	deadline := time.NewTicker(500 * time.Millisecond)
	defer deadline.Stop()
	for {
		projectionRevision, projectionFound, projectionErr := mongoRevision(ctx, client, config.MongoDBName, "banking-projection")
		featureRevision, featureFound, featureErr := mongoRevision(ctx, client, config.MongoDBName, "relay")
		if projectionErr != nil {
			return fmt.Errorf("check banking projection: %w", projectionErr)
		}
		if featureErr != nil {
			return fmt.Errorf("check model feature progress: %w", featureErr)
		}
		if projectionFound && featureFound && projectionRevision >= targetRevision && featureRevision >= targetRevision {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("projection/feature state did not reach revision %d", targetRevision)
		case <-deadline.C:
		}
	}
}

func mongoRevision(ctx context.Context, client *mongodrv.Client, database, id string) (uint64, bool, error) {
	var row struct {
		Revision       int64  `bson:"revision"`
		StreamRevision uint64 `bson:"stream_revision"`
	}
	err := client.Database(database).Collection(map[string]string{
		"banking-projection": "checkpoints",
		"relay":              "model_feature_progress",
	}[id]).FindOne(ctx, bson.M{"_id": id}).Decode(&row)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if id == "relay" {
		return row.StreamRevision, true, nil
	}
	if row.Revision < 0 {
		return 0, false, errors.New("MongoDB checkpoint revision is negative")
	}
	return uint64(row.Revision), true, nil
}

func writeManifest(path, datasetID, status string, accounts []importedAccount) error {
	manifest := importManifest{
		SchemaVersion: "1", DatasetID: datasetID, Status: status, ImportedAt: time.Now().UTC().Format(time.RFC3339), Accounts: accounts,
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	_, err = file.Write(append(content, '\n'))
	return err
}
