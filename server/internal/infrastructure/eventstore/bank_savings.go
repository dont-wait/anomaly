package eventstore

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

var (
	ErrSavingsNotFound              = errors.New("savings contract not found")
	ErrSavingsIdempotencyConflict   = errors.New("savings idempotency conflict")
	ErrSavingsQuoteUsed             = errors.New("savings opening quote already used")
	ErrSavingsSourceUnavailable     = errors.New("savings payment account unavailable")
	ErrSavingsInsufficientFunds     = errors.New("savings source balance insufficient")
	ErrSavingsWritesDisabled        = errors.New("savings event writes disabled until consumers are upgraded")
	ErrSavingsSettlementUnconfirmed = errors.New("savings settlement outcome unconfirmed")
)

// Savings event writes must be enabled ONLY after every deployed API and bank
// projector has been upgraded to decode SavingsOpened/SavingsSettled. Old
// processes cannot be repaired by shipping new code to other processes.
func savingsWritesEnabled() bool { return os.Getenv("SAVINGS_EVENT_WRITE_ENABLED") == "true" }

// mutateSavings retries only ambiguous EventStoreDB commit aborts. An aborted
// append may already have committed, so each attempt MUST re-read the canonical
// banking stream and re-run the operation's idempotency/status checks before
// issuing another append. Other errors are never blindly retried here.
func (r *BankRepository) mutateSavings(ctx context.Context, build func(*bankState) (*journal.Record, any, error)) (any, error) {
	const maxAttempts = 6
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := r.mutate(ctx, build)
		if err == nil {
			return result, nil
		}
		var esErr *esdb.Error
		if !errors.As(err, &esErr) || esErr.Code() != esdb.ErrorAborted {
			return nil, err
		}
		// In particular, a commit phase timeout does NOT imply rollback.
		// A retry is safe only through mutate(), which reloads the stream
		// before re-checking the Savings operation's stable idempotency key.
		if attempt == maxAttempts-1 {
			return nil, fmt.Errorf("savings append outcome unconfirmed after %d attempts: %w", maxAttempts, err)
		}
		backoff := time.Duration(50<<min(attempt, 3)) * time.Millisecond
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, errors.New("savings mutation retry exhausted")
}

// Financial mutation and contract transition are committed in ONE canonical
// banking-stream append. The existing mutate CAS retries against fresh state.
func (r *BankRepository) OpenSavings(ctx context.Context, in journal.SavingsOpenInput) (journal.SavingsMovement, error) {
	if !savingsWritesEnabled() {
		return journal.SavingsMovement{}, ErrSavingsWritesDisabled
	}
	if _, err := uuid.Parse(in.IdempotencyKey); err != nil {
		return journal.SavingsMovement{}, ErrSavingsIdempotencyConflict
	}
	if in.ActorID == "" || in.QuoteID == "" || in.SourceAccountID == "" || in.CustomerID == "" {
		return journal.SavingsMovement{}, savings.ErrInvalidContract
	}
	key := in.ActorID + ":" + strings.ToLower(in.IdempotencyKey)
	result, err := r.mutateSavings(ctx, func(s *bankState) (*journal.Record, any, error) {
		if id := s.SavingsKeys[key]; id != "" {
			existing := s.Savings[id]
			if existing.OpeningQuoteID != in.QuoteID || existing.Contract.SourceAccountID != in.SourceAccountID || existing.Contract.Principal != in.Principal || existing.Contract.TermMonths != in.TermMonths {
				return nil, nil, ErrSavingsIdempotencyConflict
			}
			return nil, existing, nil
		}
		if s.SavingsQuotes[in.QuoteID] != "" {
			return nil, nil, ErrSavingsQuoteUsed
		}
		now := time.Now().UTC()
		if !now.Before(in.ExpiresAt) {
			return nil, nil, ErrSavingsQuoteExpired
		}
		owner := s.Accounts[in.SourceAccountID]
		actor := s.Accounts[in.ActorID]
		if !ownedByCustomer(s.Accounts, in.ActorID, in.SourceAccountID) || actor == nil || actor.Status != account.AccountStatusActive || owner == nil || owner.CustomerId != in.CustomerID || owner.Status != account.AccountStatusActive || owner.Type != account.AccountTypePayment || owner.Currency != account.CurrencyVND {
			return nil, nil, ErrSavingsSourceUnavailable
		}
		approved, rateErr := savings.AnnualRateBpsForTerm(in.TermMonths)
		if rateErr != nil || approved != in.AnnualRateBps {
			return nil, nil, savings.ErrInvalidInterestRate
		}
		if in.PackageID != "savings_standard" {
			return nil, nil, savings.ErrInvalidContract
		}
		if in.Principal < savings.MinimumDepositVND {
			return nil, nil, savings.ErrInvalidDepositAmount
		}
		id := journal.EventID(key, "savings-contract")
		no := "SV" + strings.ToUpper(strings.ReplaceAll(id, "-", ""))[:18]
		c, err := savings.NewContract(savings.OpenContractParams{ID: id, SavingsNo: no, CustomerID: owner.CustomerId, PackageID: in.PackageID, SourceAccountID: owner.Id, Principal: in.Principal, TermMonths: in.TermMonths, AnnualRateBps: in.AnnualRateBps, OpenedAt: now})
		if err != nil {
			return nil, nil, err
		}
		accountKey := financialID(owner.Id).Hex()
		before := s.Balances[accountKey]
		if before < in.Principal {
			return nil, nil, ErrSavingsInsufficientFunds
		}
		after := before - in.Principal
		txid := journal.EventID(id, "deposit")
		m := journal.SavingsMovement{Contract: *c, ActorID: in.ActorID, OpeningQuoteID: in.QuoteID, OpeningKey: key, TransactionID: txid}
		record := &journal.Record{ID: journal.EventID(id, journal.SavingsOpened), Type: journal.SavingsOpened, At: now, AccountID: owner.Id, BalanceBefore: &before, BalanceAfter: &after, Savings: &m}
		return record, m, nil
	})
	if err != nil {
		return journal.SavingsMovement{}, err
	}
	m, ok := result.(journal.SavingsMovement)
	if !ok {
		return journal.SavingsMovement{}, fmt.Errorf("unexpected savings opening result")
	}
	return m, nil
}

var ErrSavingsQuoteExpired = errors.New("savings quote expired")

func (r *BankRepository) FindSavingsOpening(ctx context.Context, actorID, key string) (journal.SavingsMovement, bool, error) {
	s, err := r.load(ctx)
	if err != nil {
		return journal.SavingsMovement{}, false, err
	}
	id := s.SavingsKeys[actorID+":"+strings.ToLower(key)]
	if id == "" {
		return journal.SavingsMovement{}, false, nil
	}
	m := s.Savings[id]
	if !ownedByCustomer(s.Accounts, actorID, m.Contract.SourceAccountID) {
		return journal.SavingsMovement{}, false, nil
	}
	return m, true, nil
}

func (r *BankRepository) FindSavings(ctx context.Context, actorID, id string) (journal.SavingsMovement, error) {
	s, err := r.load(ctx)
	if err != nil {
		return journal.SavingsMovement{}, err
	}
	m, ok := s.Savings[id]
	if !ok || !ownedByCustomer(s.Accounts, actorID, m.Contract.SourceAccountID) || s.Accounts[actorID] == nil || s.Accounts[actorID].CustomerId != m.Contract.CustomerID {
		return journal.SavingsMovement{}, ErrSavingsNotFound
	}
	return m, nil
}

// Settlement appends one event that BOTH credits the account and settles the
// contract. The same contract can never be credited twice even with different
// idempotency keys or simultaneous requests.
func (r *BankRepository) SettleSavings(ctx context.Context, actorID, contractID, quoteID, key string, expiresAt time.Time) (journal.SavingsMovement, error) {
	if !savingsWritesEnabled() {
		return journal.SavingsMovement{}, ErrSavingsWritesDisabled
	}
	if _, err := uuid.Parse(key); err != nil {
		return journal.SavingsMovement{}, ErrSavingsIdempotencyConflict
	}
	result, err := r.mutateSavings(ctx, func(s *bankState) (*journal.Record, any, error) {
		m, exists := s.Savings[contractID]
		if !exists || !ownedByCustomer(s.Accounts, actorID, m.Contract.SourceAccountID) || s.Accounts[actorID] == nil || s.Accounts[actorID].CustomerId != m.Contract.CustomerID {
			return nil, nil, ErrSavingsNotFound
		}
		withdrawKey := actorID + ":" + strings.ToLower(key)
		if used := s.SavingsWithdrawalKeys[withdrawKey]; used != "" && used != contractID {
			return nil, nil, ErrSavingsIdempotencyConflict
		}
		if m.Contract.Status == savings.ContractSettled {
			if m.WithdrawalKey != withdrawKey || m.WithdrawalQuoteID != quoteID {
				return nil, nil, savings.ErrAlreadySettled
			}
			return nil, m, nil
		}
		owner := s.Accounts[m.Contract.SourceAccountID]
		if owner == nil || s.Accounts[actorID].Status != account.AccountStatusActive || owner.Status != account.AccountStatusActive || owner.Type != account.AccountTypePayment || owner.Currency != account.CurrencyVND {
			return nil, nil, ErrSavingsSourceUnavailable
		}
		now := time.Now().UTC()
		if !now.Before(expiresAt) {
			return nil, nil, ErrSavingsQuoteExpired
		}
		c := m.Contract
		c.Revision = s.Revision
		intent, err := c.PrepareWithdrawal(now)
		if err != nil {
			return nil, nil, err
		}
		accountKey := financialID(owner.Id).Hex()
		before := s.Balances[accountKey]
		if before < 0 || intent.TotalAmount > math.MaxInt64-before {
			return nil, nil, savings.ErrAmountOverflow
		}
		after := before + intent.TotalAmount
		txID := journal.EventID(c.ID, "withdrawal")
		if err := c.ConfirmSettled(intent, txID, s.Revision+1, now); err != nil {
			return nil, nil, err
		}
		m.Contract = c
		m.WithdrawalKey = withdrawKey
		m.WithdrawalQuoteID = quoteID
		m.TransactionID = txID
		record := &journal.Record{ID: journal.EventID(c.ID, journal.SavingsSettled), Type: journal.SavingsSettled, At: now, AccountID: owner.Id, BalanceBefore: &before, BalanceAfter: &after, Savings: &m}
		return record, m, nil
	})
	if err != nil {
		return journal.SavingsMovement{}, err
	}
	if _, ok := result.(journal.SavingsMovement); !ok {
		return journal.SavingsMovement{}, fmt.Errorf("unexpected savings settlement result")
	}
	// EventStoreDB can acknowledge a repeated EventID as an idempotent append,
	// even if this caller supplied a different withdrawal idempotency key.
	// The persisted Banking Stream, not the locally built movement, decides
	// which request actually settled the contract. Verify every success before
	// returning a receipt, including results recovered after commit timeouts.
	return confirmSavingsSettlementWithRecovery(ctx, r.load, contractID, quoteID, actorID+":"+strings.ToLower(key))
}

// confirmSavingsSettlement must never report a different request's successful
// append as this request's success. The same event ID intentionally protects
// each contract against duplicate financial credits, but it alone cannot
// distinguish two different requests during concurrent event-store appends.
func confirmSavingsSettlement(s *bankState, contractID, quoteID, withdrawKey string) (journal.SavingsMovement, error) {
	if s == nil {
		return journal.SavingsMovement{}, fmt.Errorf("savings settlement cannot be confirmed: missing bank state")
	}
	committed, found := s.Savings[contractID]
	if !found || committed.Contract.Status != savings.ContractSettled || committed.Contract.Settlement == nil {
		return journal.SavingsMovement{}, fmt.Errorf("savings settlement not yet confirmed in bank stream")
	}
	if committed.WithdrawalKey != withdrawKey || committed.WithdrawalQuoteID != quoteID {
		return journal.SavingsMovement{}, savings.ErrAlreadySettled
	}
	return committed, nil
}

// confirmSavingsSettlementWithRecovery reads the authoritative bank stream,
// including after a cancelled HTTP request. An accepted append may have
// credited money already: never re-append based on a failed confirmation read.
// Returning ErrSavingsSettlementUnconfirmed means *unknown*, not rollback.
func confirmSavingsSettlementWithRecovery(ctx context.Context, read func(context.Context) (*bankState, error), contractID, quoteID, withdrawKey string) (journal.SavingsMovement, error) {
	const maxReads = 4
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < maxReads; attempt++ {
		authoritative, err := read(recoveryCtx)
		if err == nil {
			var movement journal.SavingsMovement
			movement, err = confirmSavingsSettlement(authoritative, contractID, quoteID, withdrawKey)
			if err == nil || errors.Is(err, savings.ErrAlreadySettled) {
				return movement, err
			}
		}
		lastErr = err
		if attempt == maxReads-1 {
			break
		}
		timer := time.NewTimer(time.Duration(50<<attempt) * time.Millisecond)
		select {
		case <-recoveryCtx.Done():
			timer.Stop()
			return journal.SavingsMovement{}, fmt.Errorf("%w: %v", ErrSavingsSettlementUnconfirmed, lastErr)
		case <-timer.C:
		}
	}
	return journal.SavingsMovement{}, fmt.Errorf("%w: %v", ErrSavingsSettlementUnconfirmed, lastErr)
}

// SavingsMaturityEligible checks EventStoreDB rather than relying on a lagging
// MongoDB projection. A missing, foreign or settled contract is never eligible.
func (r *BankRepository) SavingsMaturityEligible(ctx context.Context, customerID, contractID string) (bool, error) {
	state, err := r.load(ctx)
	if err != nil {
		return false, err
	}
	m, exists := state.Savings[contractID]
	return exists && m.Contract.CustomerID == customerID && (m.Contract.Status == savings.ContractActive || m.Contract.Status == savings.ContractMatured) && !time.Now().UTC().Before(m.Contract.MaturityAt), nil
}
