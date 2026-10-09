package eventstore

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// One banking stream gives the MVP a single optimistic concurrency boundary:
// registration uniqueness, funds checks and both sides of a transfer commit
// against the same revision. Mongo is used only for paginated read projections.
type BankRepository struct {
	*mongorepo.TransferRepository
	client *esdb.Client
}

func NewBankRepository(client *esdb.Client, reads *mongorepo.TransferRepository) *BankRepository {
	return &BankRepository{client: client, TransferRepository: reads}
}

type transferState struct {
	Tx    mongorepo.TransferDocument
	Event journal.Event
}
type bankState struct {
	Accounts  map[string]*accountdomain.UserAccount
	Balances  map[string]int64
	Transfers map[string]transferState
	Keys      map[string]string
	Exists    bool
	Revision  uint64
}

func financialID(id string) bson.ObjectID {
	result, _ := bson.ObjectIDFromHex(journal.FinancialAccountID(id))
	return result
}

func (r *BankRepository) load(ctx context.Context) (*bankState, error) {
	s := &bankState{Accounts: map[string]*accountdomain.UserAccount{}, Balances: map[string]int64{}, Transfers: map[string]transferState{}, Keys: map[string]string{}}
	var from esdb.StreamPosition = esdb.Start{}
	for {
		stream, err := r.client.ReadStream(ctx, journal.BankStream, esdb.ReadStreamOptions{From: from, RequiresLeader: true}, PAGESIZE)
		if err != nil {
			if resourceMissing(err) && !s.Exists {
				return s, nil
			}
			return nil, err
		}
		count := 0
		for {
			e, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				stream.Close()
				if resourceMissing(err) && !s.Exists {
					return s, nil
				}
				return nil, err
			}
			record, err := journal.DecodeRecord(e.Event.Data, e.Event.EventID.String(), e.Event.EventType)
			if err != nil {
				stream.Close()
				return nil, err
			}
			if err := s.apply(record); err != nil {
				stream.Close()
				return nil, err
			}
			s.Exists, s.Revision = true, e.Event.EventNumber
			count++
		}
		stream.Close()
		if count < PAGESIZE {
			return s, nil
		}
		from = esdb.Revision(s.Revision + 1)
	}
}

func resourceMissing(err error) bool {
	var esErr *esdb.Error
	return errors.As(err, &esErr) && esErr.Code() == esdb.ErrorCodeResourceNotFound
}

func (s *bankState) apply(record journal.Record) error {
	switch record.Type {
	case journal.AccountCreated:
		if _, exists := s.Accounts[record.Account.Id]; exists {
			return fmt.Errorf("duplicate account creation")
		}
		s.Accounts[record.Account.Id] = record.Account
		s.Balances[financialID(record.Account.Id).Hex()] = record.Account.Balance.Current
	case journal.AccountUpdated:
		if s.Accounts[record.Account.Id] == nil {
			return accountdomain.ErrAccountNotFound
		}
		s.Accounts[record.Account.Id] = record.Account
	case journal.BalanceSeeded:
		if s.Accounts[record.AccountID] == nil {
			return accountdomain.ErrAccountNotFound
		}
		s.Balances[financialID(record.AccountID).Hex()] = *record.Balance
	case journal.BankImported:
		if s.Exists || len(s.Accounts) != 0 {
			return fmt.Errorf("bank import must be the first record")
		}
		for _, a := range record.Accounts {
			s.Accounts[a.Id] = a
			s.Balances[financialID(a.Id).Hex()] = a.Balance.Current
		}
	default:
		var tx mongorepo.TransferDocument
		if err := json.Unmarshal(record.Transfer, &tx); err != nil {
			return err
		}
		e := *record.Event
		if tx.ID != e.TransactionID || tx.Source.AccountID.Hex() != e.Payload.SourceAccountID || tx.Destination.AccountID.Hex() != e.Payload.DestinationAccountID || tx.Status != e.Payload.Status || tx.Amount != e.Payload.Amount || tx.Fee != e.Payload.Fee || tx.Currency != e.Payload.Currency || tx.Channel != e.Payload.Channel {
			return fmt.Errorf("transfer snapshot does not match event")
		}
		previous, exists := s.Transfers[tx.ID]
		if e.Type == journal.Created {
			if exists || s.Keys[tx.IdempotencyKey] != "" {
				return fmt.Errorf("duplicate transfer creation")
			}
			s.Keys[tx.IdempotencyKey] = tx.ID
		} else if !exists || previous.Tx.Status != "awaiting_otp" || e.Sequence != previous.Event.Sequence+1 {
			return fmt.Errorf("invalid transfer transition")
		}
		if e.Type == journal.Completed && !record.Historical {
			if s.Balances[e.Payload.SourceAccountID] != *e.Payload.SourceBalanceBefore || s.Balances[e.Payload.DestinationAccountID] != *e.Payload.DestinationBalanceBefore {
				return fmt.Errorf("transfer does not match canonical balances")
			}
			s.Balances[e.Payload.SourceAccountID], s.Balances[e.Payload.DestinationAccountID] = *e.Payload.SourceBalanceAfter, *e.Payload.DestinationBalanceAfter
		}
		s.Transfers[tx.ID] = transferState{Tx: tx, Event: e}
	}
	return nil
}

func (r *BankRepository) mutate(ctx context.Context, build func(*bankState) (*journal.Record, any, error)) (any, error) {
	for range 32 {
		s, err := r.load(ctx)
		if err != nil {
			return nil, err
		}
		record, result, err := build(s)
		if err != nil || record == nil {
			return result, err
		}
		data, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		if _, err := journal.DecodeRecord(data, record.ID, record.Type); err != nil {
			return nil, err
		}
		var expected esdb.StreamState = esdb.NoStream{}
		if s.Exists {
			expected = esdb.Revision(s.Revision)
		}
		_, err = r.client.AppendToStream(ctx, journal.BankStream, esdb.AppendToStreamOptions{StreamState: expected},
			esdb.EventData{EventID: uuid.MustParse(record.ID), EventType: record.Type, ContentType: esdb.ContentTypeJson, Data: data})
		if err == nil {
			return result, nil
		}
		var esErr *esdb.Error
		if !errors.As(err, &esErr) || esErr.Code() != esdb.ErrorCodeWrongExpectedVersion {
			return nil, err
		}
		// Rebuild and revalidate funds/uniqueness after every concurrent append.
	}
	return nil, mongorepo.ErrTransactionConflict
}

func (r *BankRepository) Create(ctx context.Context, a *accountdomain.UserAccount) error {
	_, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		for _, existing := range s.Accounts {
			if existing.Id == a.Id || existing.Email == a.Email || existing.Username == a.Username || existing.Customer.Identity.Number == a.Customer.Identity.Number {
				return nil, nil, accountdomain.ErrUserAlreadyExists
			}
		}
		if a.Balance.Current != 0 {
			return nil, nil, fmt.Errorf("new account must start at zero")
		}
		sum := sha256.Sum256([]byte(a.Id))
		a.AccountNo = fmt.Sprint(uint64(10000000000000) + binary.BigEndian.Uint64(sum[:8])%90000000000000)
		for _, existing := range s.Accounts {
			if existing.AccountNo == a.AccountNo {
				return nil, nil, accountdomain.ErrUserAlreadyExists
			}
		}
		return &journal.Record{ID: journal.EventID(a.Id, journal.AccountCreated), Type: journal.AccountCreated, At: a.CreatedAt, Account: a}, nil, nil
	})
	return err
}

func (r *BankRepository) Save(ctx context.Context, a *accountdomain.UserAccount) error {
	_, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		current := s.Accounts[a.Id]
		if current == nil {
			return nil, nil, accountdomain.ErrAccountNotFound
		}
		// Financial state is owned by money events. A profile read may precede
		// a concurrent transfer, so use the current canonical balance in its snapshot.
		a.Balance.Current = s.Balances[financialID(a.Id).Hex()]
		if current.Version == a.Version {
			if current.Customer.VerifiedKYCSessionId != a.Customer.VerifiedKYCSessionId {
				return nil, nil, mongorepo.ErrTransactionConflict
			}
			return nil, nil, nil
		}
		if a.Version != current.Version+1 {
			return nil, nil, mongorepo.ErrTransactionConflict
		}
		return &journal.Record{ID: journal.EventID(a.Id, fmt.Sprintf("profile:%d", a.Version)), Type: journal.AccountUpdated, At: a.UpdatedAt, Account: a}, nil, nil
	})
	return err
}

func (r *BankRepository) FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error) {
	s, err := r.load(ctx)
	if err != nil {
		return nil, err
	}
	return s.account(id)
}

func (s *bankState) account(id string) (*accountdomain.UserAccount, error) {
	a := s.Accounts[id]
	if a == nil {
		return nil, nil
	}
	data, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	var copy accountdomain.UserAccount
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}
	copy.Balance.Current = s.Balances[financialID(id).Hex()]
	return &copy, nil
}

func (r *BankRepository) FindAll(ctx context.Context) ([]*accountdomain.UserAccount, error) {
	s, err := r.load(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(s.Accounts))
	for id := range s.Accounts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*accountdomain.UserAccount, 0, len(ids))
	for _, id := range ids {
		a, err := s.account(id)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, nil
}

func (r *BankRepository) findAccount(ctx context.Context, match func(*accountdomain.UserAccount) bool) (*accountdomain.UserAccount, error) {
	accounts, err := r.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range accounts {
		if match(a) {
			return a, nil
		}
	}
	return nil, nil
}

func (r *BankRepository) FindByEmail(ctx context.Context, email string) (*accountdomain.UserAccount, error) {
	return r.findAccount(ctx, func(a *accountdomain.UserAccount) bool { return a.Email == email })
}

func (r *BankRepository) FindByUsername(ctx context.Context, name string) (*accountdomain.UserAccount, error) {
	return r.findAccount(ctx, func(a *accountdomain.UserAccount) bool { return a.Username == name })
}

func (r *BankRepository) FindByCCCDNumber(ctx context.Context, number string) (*accountdomain.UserAccount, error) {
	return r.findAccount(ctx, func(a *accountdomain.UserAccount) bool { return a.Customer.Identity.Number == number })
}

func transferAccount(a *accountdomain.UserAccount) (mongorepo.TransferAccount, error) {
	if a == nil {
		return mongorepo.TransferAccount{}, mongorepo.ErrTransactionAccountNotFound
	}
	customerID, err := bson.ObjectIDFromHex(a.CustomerId)
	if err != nil {
		return mongorepo.TransferAccount{}, err
	}
	return mongorepo.TransferAccount{ID: financialID(a.Id), Customer: customerID, AccountNo: a.AccountNo, Name: a.Customer.Profile.FullName, Email: a.Email, Balance: a.Balance.Current, Version: a.Version, Status: string(a.Status)}, nil
}

func (r *BankRepository) FindOwnedAccount(ctx context.Context, id string) (mongorepo.TransferAccount, error) {
	a, err := r.FindByID(ctx, id)
	if err != nil {
		return mongorepo.TransferAccount{}, err
	}
	return transferAccount(a)
}

func (r *BankRepository) FindRecipient(ctx context.Context, no string) (mongorepo.TransferAccount, error) {
	a, err := r.findAccount(ctx, func(a *accountdomain.UserAccount) bool {
		return a.AccountNo == no && a.Status == accountdomain.AccountStatusActive
	})
	if err != nil {
		return mongorepo.TransferAccount{}, err
	}
	return transferAccount(a)
}

func (r *BankRepository) AccountObjectID(ctx context.Context, id string) (bson.ObjectID, error) {
	a, err := r.FindOwnedAccount(ctx, id)
	return a.ID, err
}

func eventFor(tx mongorepo.TransferDocument, eventType string, sequence int64, at time.Time) journal.Event {
	return journal.Event{
		ID: journal.EventID(tx.ID, eventType), TransactionID: tx.ID, Type: eventType, SchemaVersion: journal.SchemaVersion, Sequence: sequence, OccurredAt: at,
		Payload: journal.Payload{SourceAccountID: tx.Source.AccountID.Hex(), DestinationAccountID: tx.Destination.AccountID.Hex(), Amount: tx.Amount, Fee: tx.Fee, Currency: tx.Currency, Channel: tx.Channel, Status: tx.Status},
	}
}

func transferRecord(tx mongorepo.TransferDocument, e journal.Event) (*journal.Record, error) {
	data, err := json.Marshal(tx)
	if err != nil {
		return nil, err
	}
	return &journal.Record{ID: e.ID, Type: e.Type, At: e.OccurredAt, Event: &e, Transfer: data}, nil
}

func sameRequest(a, b mongorepo.TransferDocument) bool {
	return a.Source.AccountID == b.Source.AccountID && a.Destination.AccountID == b.Destination.AccountID && a.Amount == b.Amount && a.Fee == b.Fee && a.Currency == b.Currency && a.Note == b.Note
}

func (r *BankRepository) CreatePending(ctx context.Context, tx mongorepo.TransferDocument) (mongorepo.TransferDocument, bool, error) {
	tx.IdempotencyKey = strings.ToLower(tx.IdempotencyKey)
	type creation struct {
		tx      mongorepo.TransferDocument
		created bool
	}
	result, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		if id := s.Keys[tx.IdempotencyKey]; id != "" {
			old := s.Transfers[id].Tx
			if !sameRequest(old, tx) {
				return nil, nil, mongorepo.ErrTransactionConflict
			}
			return nil, creation{old, false}, nil
		}
		if tx.Amount < 1000 || tx.Fee < 0 || tx.Source.AccountID == tx.Destination.AccountID {
			return nil, nil, mongorepo.ErrTransactionConflict
		}
		if tx.Amount > math.MaxInt64-tx.Fee || s.Balances[tx.Source.AccountID.Hex()] < tx.Amount+tx.Fee {
			return nil, nil, mongorepo.ErrTransactionInsufficient
		}
		if !s.active(tx.Source.AccountID) || !s.active(tx.Destination.AccountID) {
			return nil, nil, mongorepo.ErrTransactionAccountNotFound
		}
		record, err := transferRecord(tx, eventFor(tx, journal.Created, 1, tx.CreatedAt))
		return record, creation{tx, true}, err
	})
	if err != nil {
		return mongorepo.TransferDocument{}, false, err
	}
	created := result.(creation)
	return created.tx, created.created, nil
}

func (s *bankState) active(id bson.ObjectID) bool {
	for _, a := range s.Accounts {
		if financialID(a.Id) == id {
			return a.Status == accountdomain.AccountStatusActive
		}
	}
	return false
}

func (r *BankRepository) FindTransfer(ctx context.Context, id, owner string) (mongorepo.TransferDocument, error) {
	s, err := r.load(ctx)
	if err != nil {
		return mongorepo.TransferDocument{}, err
	}
	tx, ok := s.Transfers[id]
	if !ok || s.Accounts[owner] == nil || tx.Tx.Source.AccountID != financialID(owner) {
		return mongorepo.TransferDocument{}, mongorepo.ErrTransactionAccountNotFound
	}
	return tx.Tx, nil
}

func (r *BankRepository) FindTransferByIdempotencyKey(ctx context.Context, key, owner string) (mongorepo.TransferDocument, error) {
	s, err := r.load(ctx)
	if err != nil {
		return mongorepo.TransferDocument{}, err
	}
	return s.transferByIdempotencyKey(key, owner)
}

func (s *bankState) transferByIdempotencyKey(key, owner string) (mongorepo.TransferDocument, error) {
	id := s.Keys[strings.ToLower(key)]
	tx, ok := s.Transfers[id]
	if !ok || s.Accounts[owner] == nil || tx.Tx.Source.AccountID != financialID(owner) {
		return mongorepo.TransferDocument{}, mongorepo.ErrTransactionAccountNotFound
	}
	return tx.Tx, nil
}

func (r *BankRepository) ConfirmTransfer(ctx context.Context, id, owner string) (mongorepo.TransferDocument, error) {
	result, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		state, ok := s.Transfers[id]
		tx := state.Tx
		if !ok || s.Accounts[owner] == nil || tx.Source.AccountID != financialID(owner) {
			return nil, nil, mongorepo.ErrTransactionAccountNotFound
		}
		if tx.Status == "success" {
			return nil, tx, nil
		}
		if tx.Status != "awaiting_otp" {
			return nil, nil, mongorepo.ErrTransactionConflict
		}
		if !s.active(tx.Source.AccountID) || !s.active(tx.Destination.AccountID) {
			return nil, nil, mongorepo.ErrTransactionAccountNotFound
		}
		source, destination := s.Balances[tx.Source.AccountID.Hex()], s.Balances[tx.Destination.AccountID.Hex()]
		if tx.Amount > math.MaxInt64-tx.Fee || destination > math.MaxInt64-tx.Amount {
			return nil, nil, mongorepo.ErrTransactionBalanceOverflow
		}
		if source < tx.Amount+tx.Fee {
			return nil, nil, mongorepo.ErrTransactionInsufficient
		}
		newSource, newDestination := source-tx.Amount-tx.Fee, destination+tx.Amount
		tx.Status, tx.PostedAt = "success", time.Now().UTC()
		tx.Authentication.Method, tx.Authentication.Verified = "email_otp", true
		e := eventFor(tx, journal.Completed, state.Event.Sequence+1, tx.PostedAt)
		e.Payload.SourceBalanceBefore, e.Payload.SourceBalanceAfter = &source, &newSource
		e.Payload.DestinationBalanceBefore, e.Payload.DestinationBalanceAfter = &destination, &newDestination
		record, err := transferRecord(tx, e)
		return record, tx, err
	})
	if err != nil {
		return mongorepo.TransferDocument{}, err
	}
	return result.(mongorepo.TransferDocument), nil
}

func (r *BankRepository) CancelPending(ctx context.Context, id string) error {
	_, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		state, ok := s.Transfers[id]
		if !ok || state.Tx.Status != "awaiting_otp" {
			return nil, nil, nil
		}
		tx := state.Tx
		tx.Status = "cancelled"
		record, err := transferRecord(tx, eventFor(tx, journal.Cancelled, state.Event.Sequence+1, time.Now().UTC()))
		return record, nil, err
	})
	return err
}

func (r *BankRepository) SetOTPMetadata(ctx context.Context, id string, expires, resend time.Time) error {
	_, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		state, ok := s.Transfers[id]
		if !ok {
			return nil, nil, mongorepo.ErrTransactionAccountNotFound
		}
		if state.Tx.Status != "awaiting_otp" {
			return nil, nil, mongorepo.ErrTransactionConflict
		}
		tx := state.Tx
		if tx.OTPExpiresAt.Equal(expires) && tx.OTPResendAt.Equal(resend) {
			return nil, nil, nil
		}
		tx.OTPExpiresAt, tx.OTPResendAt = expires, resend
		e := eventFor(tx, journal.OTPUpdated, state.Event.Sequence+1, time.Now().UTC())
		e.ID = journal.EventID(id, "otp:"+expires.UTC().Format(time.RFC3339Nano))
		record, err := transferRecord(tx, e)
		return record, nil, err
	})
	return err
}

func (r *BankRepository) GetFeed(ctx context.Context, accountID bson.ObjectID, id string) (mongorepo.FeedDocument, error) {
	s, err := r.load(ctx)
	if err != nil {
		return mongorepo.FeedDocument{}, err
	}
	state, ok := s.Transfers[id]
	tx := state.Tx
	if !ok || (tx.Source.AccountID != accountID && tx.Destination.AccountID != accountID) || (tx.Status != "success" && tx.Status != "cancelled") || (tx.Status == "cancelled" && tx.Source.AccountID != accountID) {
		return mongorepo.FeedDocument{}, mongorepo.ErrTransactionAccountNotFound
	}
	feed := mongorepo.FeedDocument{AccountID: accountID, TransactionID: id, Direction: "OUT", Type: tx.Type, Amount: tx.Amount, Fee: tx.Fee, Status: tx.Status, Note: tx.Note, Reference: tx.Reference, OccurredAt: state.Event.OccurredAt}
	feed.Counterparty.AccountNo, feed.Counterparty.Name, feed.Counterparty.BankCode = tx.Destination.AccountNo, tx.Destination.Name, tx.Destination.BankCode
	feed.BalanceAfter = state.Event.Payload.SourceBalanceAfter
	if tx.Destination.AccountID == accountID {
		feed.Direction, feed.Fee, feed.BalanceAfter = "IN", 0, state.Event.Payload.DestinationBalanceAfter
		feed.Counterparty.AccountNo, feed.Counterparty.Name, feed.Counterparty.BankCode = tx.Source.AccountNo, tx.Source.Name, "ANOMALY"
	}
	return feed, nil
}

// Only the opt-in development seeder calls this; production transfers cannot
// set balances. Funding is still recorded in the canonical stream.
func (r *BankRepository) SetSeedBalance(ctx context.Context, id string, balance int64) error {
	operationID := uuid.NewString()
	_, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		if s.Accounts[id] == nil || balance < 0 {
			return nil, nil, accountdomain.ErrAccountNotFound
		}
		if s.Balances[financialID(id).Hex()] == balance {
			return nil, nil, nil
		}
		return &journal.Record{ID: operationID, Type: journal.BalanceSeeded, At: time.Now().UTC(), AccountID: id, Balance: &balance}, nil, nil
	})
	return err
}

type SeedRepository struct{ *BankRepository }

func (r SeedRepository) Save(ctx context.Context, a *accountdomain.UserAccount) error {
	return r.SetSeedBalance(ctx, a.Id, a.Balance.Current)
}

// EnsureReady prevents starting a fresh canonical history over existing Mongo
// business data accidentally. Empty installations can register their first account.
func (r *BankRepository) EnsureReady(ctx context.Context, hasMongoAccounts bool) error {
	state, err := r.load(ctx)
	if err != nil {
		return err
	}
	if !state.Exists {
		if hasMongoAccounts {
			return fmt.Errorf("banking stream missing while Mongo contains accounts; perform the explicit offline cutover first")
		}
		legacy, err := NewAccountRepository(r.client).allAccountIDs(ctx)
		if err != nil {
			return err
		}
		if len(legacy) > 0 {
			return fmt.Errorf("canonical legacy account streams found; migrate them before starting new banking commands")
		}
	}
	return nil
}
