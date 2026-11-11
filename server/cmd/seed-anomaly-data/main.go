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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
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
	ImportedAt    string            `json:"imported_at"`
	Accounts      []importedAccount `json:"accounts"`
}

func main() {
	log := logger.NewLogger(zerolog.InfoLevel)
	file := flag.String("file", "../tools/test-data/output/anomaly-a-f.json", "generated anomaly fixture JSON")
	manifest := flag.String("manifest", "", "optional output path for actual account numbers and credentials")
	flag.Parse()

	if err := validateEnvironment(); err != nil {
		log.Fatal().Err(err).Msg("anomaly fixture import refused")
	}

	data, err := readDataset(*file)
	if err != nil {
		log.Fatal().Err(err).Msg("read anomaly fixture failed")
	}
	if data.DatasetID == "" || len(data.Accounts) == 0 {
		log.Fatal().Msg("anomaly fixture is empty or missing dataset_id")
	}

	loader := domain.GetEnvLoader().Load(log)
	config := loader.LoadEventStoreConfig()
	client, err := eventstore.NewEventStoreClient(config)
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore failed")
	}
	defer eventstore.Disconnect(client)

	repo := eventstore.SeedRepository{BankRepository: eventstore.NewBankRepository(client, nil)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	accounts, err := importDataset(ctx, &repo, data)
	if err != nil {
		log.Fatal().Err(err).Msg("import anomaly fixture failed")
	}

	manifestPath := *manifest
	if manifestPath == "" {
		manifestPath = filepath.Join(filepath.Dir(*file), "anomaly-a-f.imported.json")
	}
	if err := writeManifest(manifestPath, data.DatasetID, accounts); err != nil {
		log.Fatal().Err(err).Msg("write anomaly fixture manifest failed")
	}

	fmt.Printf("Imported dataset %s\n", data.DatasetID)
	for _, account := range accounts {
		fmt.Printf("%s\t%s\tCCCD=%s\tpassword=%s\taccount_no=%s\thistory=%d\n", account.Alias, account.Case, account.CCCD, account.Password, account.AccountNo, account.HistoryCount)
	}
	fmt.Printf("Manifest: %s\n", manifestPath)
}

func validateEnvironment() error {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv != "development" && appEnv != "dev" && appEnv != "local" {
		return fmt.Errorf("APP_ENV must be development, dev, or local; got %q", appEnv)
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("SEED_DEMO_ENABLED")), "true") {
		return errors.New("SEED_DEMO_ENABLED=true is required")
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

func importDataset(ctx context.Context, repo *eventstore.SeedRepository, data dataset) ([]importedAccount, error) {
	accountIDs := make(map[string]string, len(data.Accounts))
	accountNumbers := make(map[string]string, len(data.Accounts))
	imported := make([]importedAccount, 0, len(data.Accounts))

	for _, fixture := range data.Accounts {
		account, created, err := ensureFixtureAccount(ctx, repo, data.DatasetID, fixture)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", fixture.ID, err)
		}
		if fixture.ExpectedBalanceAfterHistory > 0 {
			current, err := repo.FindByID(ctx, account.Id)
			if err != nil {
				return nil, fmt.Errorf("read balance for %s: %w", fixture.ID, err)
			}
			if current.Balance.Current == 0 {
				if err := repo.SetSeedBalance(ctx, account.Id, fixture.ExpectedBalanceAfterHistory); err != nil {
					return nil, fmt.Errorf("fund %s: %w", fixture.ID, err)
				}
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
		accountNumbers[fixture.ID] = account.AccountNo

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

func writeManifest(path, datasetID string, accounts []importedAccount) error {
	manifest := importManifest{
		SchemaVersion: "1", DatasetID: datasetID, ImportedAt: time.Now().UTC().Format(time.RFC3339), Accounts: accounts,
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0o600)
}
