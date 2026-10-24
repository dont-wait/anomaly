package eventstore

import (
	"testing"
	"time"

	account "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func TestSavingsBankStateRejectsReplayedDebitAndCredit(t *testing.T) {
	a := &account.UserAccount{Id: "actor", CustomerId: "owner", Status: account.AccountStatusActive, Type: account.AccountTypePayment, Currency: account.CurrencyVND}
	s := &bankState{Accounts: map[string]*account.UserAccount{"actor": a}, Balances: map[string]int64{financialID(a.Id).Hex(): 2_000_000}, Savings: map[string]journal.SavingsMovement{}, SavingsKeys: map[string]string{}, SavingsQuotes: map[string]string{}, SavingsWithdrawalKeys: map[string]string{}}
	openedAt := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	c, err := savings.NewContract(savings.OpenContractParams{ID: journal.EventID("actor:key", "contract"), SavingsNo: "SVTEST", CustomerID: "owner", PackageID: "savings_standard", SourceAccountID: "actor", Principal: 1_000_000, TermMonths: 3, AnnualRateBps: 450, OpenedAt: openedAt})
	if err != nil {
		t.Fatal(err)
	}
	before, after := int64(2_000_000), int64(1_000_000)
	m := journal.SavingsMovement{Contract: *c, ActorID: "actor", OpeningQuoteID: "quote", OpeningKey: "actor:key", TransactionID: journal.EventID(c.ID, "deposit")}
	open := journal.Record{ID: journal.EventID(c.ID, journal.SavingsOpened), Type: journal.SavingsOpened, At: openedAt, AccountID: a.Id, BalanceBefore: &before, BalanceAfter: &after, Savings: &m}
	if err := s.apply(open); err != nil {
		t.Fatal(err)
	}
	if err := s.apply(open); err == nil {
		t.Fatal("allowed duplicate debit")
	}
	now := c.MaturityAt.Add(time.Second)
	c.Revision = 3
	intent, err := c.PrepareWithdrawal(now)
	if err != nil {
		t.Fatal(err)
	}
	transactionID := journal.EventID(c.ID, "withdrawal")
	if err := c.ConfirmSettled(intent, transactionID, 4, now); err != nil {
		t.Fatal(err)
	}
	m.Contract = *c
	m.WithdrawalKey = "actor:withdraw-key"
	m.WithdrawalQuoteID = "wq"
	m.TransactionID = transactionID
	before, after = 1_000_000, 1_000_000+intent.TotalAmount
	settle := journal.Record{ID: journal.EventID(c.ID, journal.SavingsSettled), Type: journal.SavingsSettled, At: now, AccountID: a.Id, BalanceBefore: &before, BalanceAfter: &after, Savings: &m}
	if err := s.apply(settle); err != nil {
		t.Fatal(err)
	}
	if s.Balances[financialID(a.Id).Hex()] != after {
		t.Fatal("bad payout balance")
	}
	if err := s.apply(settle); err == nil {
		t.Fatal("allowed duplicate credit")
	}
}
