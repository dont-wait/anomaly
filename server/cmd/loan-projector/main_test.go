package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/commands"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
)

func TestReconcileCreditsOnce_RetryAfterFailure(t *testing.T) {
	ctx := context.Background()
	creditedAt := time.Now().UTC()

	credits := []eventstore.CreditedLoan{
		{
			LoanID:     "LOAN001",
			AccountID:  "ACCOUNT001",
			Amount:     50_000_000,
			CreditedAt: creditedAt,
		},
	}

	listCalls := 0
	finalizeCalls := 0

	list := func(
		_ context.Context,
	) ([]eventstore.CreditedLoan, error) {
		listCalls++
		return credits, nil
	}

	finalize := func(
		_ context.Context,
		cmd commands.ReconcileCreditedLoanCommand,
	) error {
		finalizeCalls++

		if cmd.LoanID != "LOAN001" ||
			cmd.AccountID != "ACCOUNT001" ||
			cmd.Amount != 50_000_000 ||
			!cmd.CreditedAt.Equal(creditedAt) {
			t.Fatal("incorrect credited loan data")
		}

		if finalizeCalls == 1 {
			return errors.New("simulated append failure")
		}

		return nil
	}

	// Lần quét đầu: ghi LoanDisbursed thất bại.
	err := reconcileCreditsOnce(ctx, list, finalize)

	if err == nil {
		t.Fatal("expected first reconciliation to fail")
	}

	// Lần quét tiếp theo: phục hồi thành công.
	err = reconcileCreditsOnce(ctx, list, finalize)
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}

	if listCalls != 2 || finalizeCalls != 2 {
		t.Fatalf(
			"unexpected calls: list=%d, finalize=%d",
			listCalls,
			finalizeCalls,
		)
	}
}

func TestReconcileCreditsOnce_ContinuesAfterFailure(t *testing.T) {
	ctx := context.Background()

	credits := []eventstore.CreditedLoan{
		{
			LoanID:     "LOAN001",
			AccountID:  "ACCOUNT001",
			Amount:     50_000_000,
			CreditedAt: time.Now().UTC(),
		},
		{
			LoanID:     "LOAN002",
			AccountID:  "ACCOUNT002",
			Amount:     30_000_000,
			CreditedAt: time.Now().UTC(),
		},
	}

	processed := []string{}

	list := func(
		_ context.Context,
	) ([]eventstore.CreditedLoan, error) {
		return credits, nil
	}

	finalize := func(
		_ context.Context,
		cmd commands.ReconcileCreditedLoanCommand,
	) error {
		processed = append(processed, cmd.LoanID)

		if cmd.LoanID == "LOAN001" {
			return errors.New("simulated failure")
		}

		return nil
	}

	err := reconcileCreditsOnce(ctx, list, finalize)

	if err == nil {
		t.Fatal("expected reconciliation error")
	}

	if len(processed) != 2 {
		t.Fatalf(
			"expected both loans to be processed, got %d",
			len(processed),
		)
	}

	if processed[0] != "LOAN001" ||
		processed[1] != "LOAN002" {
		t.Fatalf("unexpected processing order: %v", processed)
	}
}
