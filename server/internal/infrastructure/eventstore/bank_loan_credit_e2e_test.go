//go:build e2e

package eventstore_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	commands "github.com/dont-wait/anomaly/internal/application/account/commands"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/google/uuid"
)

func TestLoanCreditExactlyOnceInCanonicalBankStream(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	es := startEventStore(t, ctx)
	bank := eventstore.NewBankRepository(es, nil)

	register := commands.NewRegisterAccountCommandHandler(
		bank,
		bank,
	)

	account, err := register.Handle(
		ctx,
		commands.RegisterAccountCommand{
			IdempotencyKey: uuid.NewString(),
			Username:       "loan-credit-test",
			Email:          "loan-credit-test@example.com",
			Password:       "TestBank@123",
			CCCDNumber:     "012345678912",
			CCCDIssuedDate: time.Now().Add(
				-24 * time.Hour,
			),
			DOB: time.Now().AddDate(-25, 0, 0),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	verify := commands.NewVerifyAccountCommandHandler(
		bank,
	)

	_, err = verify.Handle(
		ctx,
		commands.VerifyAccountCommand{
			AccountID: account.Id,
			IdCardFrontUrl: "kyc/" +
				account.Id + "/id-card-front/front",
			IdCardBackUrl: "kyc/" +
				account.Id + "/id-card-back/back",
			LiveVideoUrl: "kyc/" +
				account.Id + "/live-video/live",
			// Persistence fixture; this is not a biometric accuracy assertion.
			Evidence: &accountdomain.KYCEvidence{MatchingStatus: "passed", LivenessStatus: "passed", ChallengePassed: true, SubjectConsistencyPassed: true}, MatchScore: 0.9, LivenessScore: 1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	loanID := uuid.NewString()

	// Giải ngân lần đầu.
	first, err := bank.CreditLoanDisbursement(
		ctx,
		loanID,
		account.Id,
		10_000_000,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Gửi lại đúng yêu cầu cũ.
	again, err := bank.CreditLoanDisbursement(
		ctx,
		loanID,
		account.Id,
		10_000_000,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !first.Equal(again) {
		t.Fatalf(
			"credit timestamps differ: %v vs %v",
			first,
			again,
		)
	}

	owner, err := bank.FindByID(
		ctx,
		account.Id,
	)
	if err != nil {
		t.Fatal(err)
	}

	if owner.Balance.Current != 10_000_000 {
		t.Fatalf(
			"double credited: balance %d",
			owner.Balance.Current,
		)
	}

	credits := 0

	for _, event := range readBank(t, ctx, es) {
		if event.Type == journal.LoanDisbursementCredited {
			credits++

			if event.LoanID != loanID ||
				event.AccountID != account.Id {
				t.Fatalf(
					"unexpected loan credit record: %+v",
					event,
				)
			}
		}
	}

	if credits != 1 {
		t.Fatal(
			fmt.Sprintf(
				"expected exactly one credit event, got %d",
				credits,
			),
		)
	}
}
