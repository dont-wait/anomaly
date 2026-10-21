package eventstore

import (
	"context"
	"fmt"
	"sort"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
)

// ListLoanPayments doc cac ky da thanh toan tu Banking Stream.
func (r *BankRepository) ListLoanPayments(
	ctx context.Context,
	loanID string,
) ([]journal.LoanInstallmentPayment, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("bank repository unavailable")
	}
	if loanID == "" {
		return nil, fmt.Errorf("loan ID is required")
	}

	state, err := r.load(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]journal.LoanInstallmentPayment, 0)

	for _, p := range state.LoanPayments {
		if p.LoanID == loanID {
			result = append(result, p)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].InstallmentNo < result[j].InstallmentNo
	})

	return result, nil
}

// DebitLoanInstallment tru dung so tien cua mot ky.
//
// Idempotency theo loanID + installmentNo.
// Neu da thu ky nay, tra lai ket qua cu.
// Banking Stream la noi quyet dinh so du chinh thuc.
func (r *BankRepository) DebitLoanInstallment(
	ctx context.Context,
	p journal.LoanInstallmentPayment,
) (journal.LoanInstallmentPayment, error) {
	if r == nil || r.client == nil {
		return journal.LoanInstallmentPayment{},
			fmt.Errorf("bank repository unavailable")
	}

	if p.LoanID == "" ||
		p.AccountID == "" ||
		p.CustomerID == "" ||
		p.InstallmentNo <= 0 ||
		p.PrincipalAmount <= 0 ||
		p.InterestAmount < 0 ||
		p.Amount <= 0 ||
		p.PrincipalAmount > p.Amount ||
		p.InterestAmount != p.Amount-p.PrincipalAmount ||
		p.OutstandingPrincipalAfter < 0 ||
		p.DueDate.IsZero() {
		return journal.LoanInstallmentPayment{},
			fmt.Errorf("invalid repayment request")
	}

	txID := journal.LoanInstallmentTransactionID(
		p.LoanID,
		p.InstallmentNo,
	)

	p.TransactionID = txID
	p.PaymentID = journal.EventID(txID, "payment")

	result, err := r.mutate(
		ctx,
		func(s *bankState) (*journal.Record, any, error) {
			key := fmt.Sprintf(
				"%s:%d",
				p.LoanID,
				p.InstallmentNo,
			)

			if previous, exists := s.LoanPayments[key]; exists {
				if previous.AccountID != p.AccountID ||
					previous.CustomerID != p.CustomerID ||
					previous.Amount != p.Amount ||
					previous.PrincipalAmount != p.PrincipalAmount ||
					previous.InterestAmount != p.InterestAmount ||
					previous.OutstandingPrincipalAfter !=
						p.OutstandingPrincipalAfter ||
					!previous.DueDate.Equal(p.DueDate) {

					return nil, nil, fmt.Errorf(
						"loan repayment idempotency conflict",
					)
				}

				return nil, previous, nil
			}

			credit, exists := s.LoanCredits[p.LoanID]
			if !exists ||
				!ownedByCustomer(s.Accounts, p.CustomerID, credit.AccountID) ||
				!ownedByCustomer(s.Accounts, p.CustomerID, p.AccountID) {
				return nil, nil, fmt.Errorf(
					"loan disbursement or payment account ownership mismatch",
				)
			}

			// Kiem tra ky truoc va du no.
			openingPrincipal := credit.Amount

			if p.InstallmentNo > 1 {
				previousKey := fmt.Sprintf(
					"%s:%d",
					p.LoanID,
					p.InstallmentNo-1,
				)

				previous, ok := s.LoanPayments[previousKey]
				if !ok {
					return nil, nil, fmt.Errorf(
						"previous installment not paid",
					)
				}

				openingPrincipal = previous.OutstandingPrincipalAfter
			}

			if openingPrincipal !=
				p.OutstandingPrincipalAfter+p.PrincipalAmount {
				return nil, nil, fmt.Errorf(
					"outstanding principal mismatch",
				)
			}

			owner := s.Accounts[p.AccountID]

			if owner == nil ||
				!ownedByCustomer(s.Accounts, p.CustomerID, p.AccountID) ||
				owner.Status != accountdomain.AccountStatusActive ||
				owner.Type != accountdomain.AccountTypePayment ||
				owner.Currency != accountdomain.CurrencyVND ||
				!owner.IsVerified() {
				return nil, nil, fmt.Errorf(
					"invalid customer payment account",
				)
			}

			at := time.Now().UTC()
			p.PaidAt = at

			// Kiem tra dung ngay den han tai thoi diem ghi no.
			if err := p.Validate(); err != nil {
				return nil, nil, err
			}

			accountKey := financialID(p.AccountID).Hex()
			before := s.Balances[accountKey]

			if before < p.Amount {
				return nil, nil,
					accountdomain.ErrInsufficientFunds
			}

			after := before - p.Amount

			record := &journal.Record{
				ID: journal.EventID(
					p.TransactionID,
					journal.LoanInstallmentPaid,
				),
				Type:          journal.LoanInstallmentPaid,
				At:            at,
				AccountID:     p.AccountID,
				LoanID:        p.LoanID,
				BalanceBefore: &before,
				BalanceAfter:  &after,
				LoanPayment:   &p,
			}

			return record, p, nil
		},
	)
	if err != nil {
		return journal.LoanInstallmentPayment{}, err
	}

	payment, ok := result.(journal.LoanInstallmentPayment)
	if !ok {
		return journal.LoanInstallmentPayment{},
			mongorepo.ErrTransactionConflict
	}

	return payment, nil
}
