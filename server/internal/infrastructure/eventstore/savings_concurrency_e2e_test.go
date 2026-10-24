//go:build e2e

package eventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	accountcommands "github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

// Testcontainers starts a disposable EventStoreDB with a fresh banking stream.
// Verifies persisted debit/credit behavior and optimistic-concurrency retries.
func TestSavingsCommandsExactlyOnceWithEventStoreDB(t *testing.T) {
	t.Setenv("SAVINGS_EVENT_WRITE_ENABLED", "true") // test-only disposable stream
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	es := startEventStore(t, ctx)
	bank := eventstore.NewBankRepository(es, nil)
	register := accountcommands.NewRegisterAccountCommandHandler(bank, bank)
	actor, err := register.Handle(ctx, accountcommands.RegisterAccountCommand{
		IdempotencyKey: uuid.NewString(), Username: "savings-e2e", Email: "savings-e2e@example.test",
		Password: "TestBank@123", CCCDNumber: "012345678912", CCCDIssuedDate: time.Now().Add(-24 * time.Hour), DOB: time.Now().AddDate(-25, 0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	verify := accountcommands.NewVerifyAccountCommandHandler(bank)
	if _, err := verify.Handle(ctx, accountcommands.VerifyAccountCommand{
		AccountID:      actor.Id,
		IdCardFrontUrl: "kyc/" + actor.Id + "/id-card-front/front",
		IdCardBackUrl:  "kyc/" + actor.Id + "/id-card-back/back",
		LiveVideoUrl:   "kyc/" + actor.Id + "/live-video/live",
	}); err != nil {
		t.Fatal(err)
	}
	const initial int64 = 5_000_000
	if err := bank.SetSeedBalance(ctx, actor.Id, initial); err != nil {
		t.Fatal(err)
	}
	owner, err := bank.FindByID(ctx, actor.Id)
	if err != nil {
		t.Fatal(err)
	}
	openID := uuid.NewString()
	quoteID := uuid.NewString()
	request := journal.SavingsOpenInput{
		ActorID: actor.Id, CustomerID: owner.CustomerId, SourceAccountID: actor.Id,
		QuoteID: quoteID, IdempotencyKey: openID, PackageID: "savings_standard",
		Principal: 1_000_000, TermMonths: 3, AnnualRateBps: 450, ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	// Concurrent duplicate requests must never create two debits.
	var wg sync.WaitGroup
	opened := make([]journal.SavingsMovement, 2)
	errs := make([]error, 2)
	for i := range opened {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			opened[index], errs[index] = eventstore.NewBankRepository(es, nil).OpenSavings(ctx, request)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent opening %d: %v", i, err)
		}
		if opened[i].Contract.ID != opened[0].Contract.ID {
			t.Fatal("idempotency produced different contracts")
		}
	}
	assertBankBalance(t, ctx, bank, actor.Id, initial-request.Principal)
	if got := countSavingsEvents(readBank(t, ctx, es), journal.SavingsOpened); got != 1 {
		t.Fatalf("opening debit events=%d, want=1", got)
	}

	// Use a historical SavingsOpened event in this isolated stream to test
	// maturity without changing the production clock or shortening terms.
	current, err := bank.FindByID(ctx, actor.Id)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().AddDate(0, -6, 0)
	contract, err := savings.NewContract(savings.OpenContractParams{
		ID: uuid.NewString(), SavingsNo: "SVHISTORICAL", CustomerID: current.CustomerId,
		PackageID: "savings_standard", SourceAccountID: actor.Id, Principal: 1_000_000,
		TermMonths: 3, AnnualRateBps: 450, OpenedAt: past,
	})
	if err != nil {
		t.Fatal(err)
	}
	pastBalance := initial - request.Principal
	afterPast := pastBalance - contract.Principal
	movement := journal.SavingsMovement{
		Contract: *contract, ActorID: actor.Id,
		OpeningQuoteID: uuid.NewString(), OpeningKey: actor.Id + ":" + uuid.NewString(),
		TransactionID: journal.EventID(contract.ID, "deposit"),
	}
	record := journal.Record{
		ID: journal.EventID(contract.ID, journal.SavingsOpened), Type: journal.SavingsOpened,
		At: past, AccountID: actor.Id, BalanceBefore: &pastBalance, BalanceAfter: &afterPast, Savings: &movement,
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	previous := readBank(t, ctx, es)
	if _, err := es.AppendToStream(ctx, journal.BankStream, esdb.AppendToStreamOptions{StreamState: esdb.Revision(uint64(len(previous) - 1))},
		esdb.EventData{EventID: uuid.MustParse(record.ID), EventType: record.Type, ContentType: esdb.ContentTypeJson, Data: data}); err != nil {
		t.Fatal(err)
	}
	assertBankBalance(t, ctx, eventstore.NewBankRepository(es, nil), actor.Id, afterPast)

	keys := []string{uuid.NewString(), uuid.NewString()}
	withdrawQuote := uuid.NewString()
	results := make([]journal.SavingsMovement, 2)
	withdrawErrors := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], withdrawErrors[index] = eventstore.NewBankRepository(es, nil).SettleSavings(
				ctx, actor.Id, contract.ID, withdrawQuote, keys[index], time.Now().Add(5*time.Minute))
		}(i)
	}
	wg.Wait()
	winner := -1
	for i, err := range withdrawErrors {
		if err == nil {
			if winner >= 0 {
				t.Fatal("two concurrent withdrawals succeeded")
			}
			winner = i
		} else if !errors.Is(err, savings.ErrAlreadySettled) {
			t.Fatalf("unexpected concurrent settlement error: %v", err)
		}
	}
	if winner < 0 {
		t.Fatalf("no successful settlement, errors=%v", withdrawErrors)
	}
	// Repository-level regression: the stream must contain the winning key,
	// not merely an idempotent acknowledgement of the shared EventID.
	persisted, err := eventstore.NewBankRepository(es, nil).FindSavings(ctx, actor.Id, contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.WithdrawalKey != actor.Id+":"+keys[winner] || persisted.WithdrawalQuoteID != withdrawQuote {
		t.Fatalf("repository returned wrong winner: saved key=%q success key=%q", persisted.WithdrawalKey, keys[winner])
	}
	payout := results[winner].Contract.Settlement.TotalAmount
	assertBankBalance(t, ctx, eventstore.NewBankRepository(es, nil), actor.Id, afterPast+payout)
	again, err := eventstore.NewBankRepository(es, nil).SettleSavings(ctx, actor.Id, contract.ID, withdrawQuote, keys[winner], time.Now().Add(5*time.Minute))
	if err != nil || again.TransactionID != results[winner].TransactionID {
		t.Fatalf("idempotent withdrawal replay=%+v, err=%v", again, err)
	}
	assertBankBalance(t, ctx, bank, actor.Id, afterPast+payout)
	if got := countSavingsEvents(readBank(t, ctx, es), journal.SavingsSettled); got != 1 {
		t.Fatalf("settlement credit events=%d, want=1", got)
	}
}

func countSavingsEvents(records []journal.Record, eventType string) int {
	count := 0
	for _, record := range records {
		if record.Type == eventType {
			count++
		}
	}
	return count
}
