package mongo

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// UUID EventStoreDB duoc anh xa thanh ObjectID on dinh.
func repaymentObjectID(kind, id string) bson.ObjectID {
	sum := sha256.Sum256(
		[]byte("anomalybank:" + kind + ":" + id),
	)

	var result bson.ObjectID
	copy(result[:], sum[:12])
	return result
}

func loanProjectionIDs(
	sourceID string,
) (bson.ObjectID, bson.ObjectID) {
	return repaymentObjectID("loan", sourceID),
		repaymentObjectID("loan-application", sourceID)
}

// Luu lai suat %/nam bang Decimal128.
// 1630 bps -> 16.30%.
func loanRateDecimal(
	bps int64,
) (bson.Decimal128, error) {
	return bson.ParseDecimal128(
		fmt.Sprintf("%d.%02d", bps/100, bps%100),
	)
}

func ensureLoanRepaymentIndexes(
	ctx context.Context,
	db *mongodrv.Database,
) error {
	indexes := []struct {
		collection string
		keys       bson.D
	}{
		{
			"loan_applications",
			bson.D{{Key: "application_no", Value: 1}},
		},
		{
			"loans",
			bson.D{{Key: "application_id", Value: 1}},
		},
		{
			"loans",
			bson.D{{Key: "loan_no", Value: 1}},
		},
		{
			"loan_payments",
			bson.D{
				{Key: "loan_id", Value: 1},
				{Key: "installment_no", Value: 1},
			},
		},
		{
			"loan_payments",
			bson.D{{Key: "transaction_id", Value: 1}},
		},
	}

	for _, item := range indexes {
		_, err := db.Collection(
			item.collection,
		).Indexes().CreateOne(
			ctx,
			mongodrv.IndexModel{
				Keys: item.keys,
				Options: options.Index().
					SetUnique(true),
			},
		)
		if err != nil {
			return fmt.Errorf(
				"create %s index: %w",
				item.collection,
				err,
			)
		}
	}

	return nil
}

func (p *BankProjection) EnsureLoanRepaymentIndexes(
	ctx context.Context,
) error {
	return ensureLoanRepaymentIndexes(ctx, p.db)
}

// Chuyen lich EMI da ky sang MongoDB.
// Khong tinh lai suat hoac EMI lan nua.
func projectedInstallments(
	items []loandomain.RepaymentScheduleItem,
) []bson.M {
	result := make([]bson.M, 0, len(items))

	for _, item := range items {
		result = append(result, bson.M{
			"installment_no": item.InstallmentNo,
			"due_date":       item.DueDate,
			"principal_due":  item.PrincipalAmount,
			"interest_due":   item.InterestAmount,
			"total_due":      item.TotalPayment,
			"status":         "upcoming",
		})
	}

	return result
}

// Tao khoan vay khi LoanDisbursed da duoc ghi vao
// EventStoreDB va Loan Projector replay thanh cong.
func (m *LoanReadModel) SaveLoanContract(
	ctx context.Context,
	a *loandomain.Application,
) error {
	if a == nil {
		return fmt.Errorf("nil loan application")
	}

	if a.Status != loandomain.ApplicationStatusDisbursed {
		return nil
	}

	repayment, err := loandomain.NewLoanRepaymentFromDisbursed(a)
	if err != nil {
		return err
	}

	accountID, err := accountRecordID(
		a.DisbursementAccountId,
	)
	if err != nil {
		return err
	}

	// Khong tu coi Application.CustomerId la
	// MongoDB customers._id.
	// Lay customer_id that tu tai khoan.
	var account struct {
		CustomerID bson.ObjectID `bson:"customer_id"`
	}

	err = m.db.Collection("accounts").FindOne(
		ctx,
		bson.M{"_id": accountID},
	).Decode(&account)
	if err != nil {
		return fmt.Errorf(
			"loan account projection missing: %w",
			err,
		)
	}

	rate, err := loanRateDecimal(
		repayment.AnnualRateBps,
	)
	if err != nil {
		return err
	}

	loanOID, appOID := loanProjectionIDs(a.Id)

	// Tao document loan_applications de
	// loans.application_id tham chieu dung ObjectId.
	appDoc := bson.M{
		"source_application_id": a.Id,
		"application_no":        a.ApplicationNo,
		"customer_id":           account.CustomerID,
		"request": bson.M{
			"amount":      a.RequestedAmount,
			"term_months": a.TermMonths,
		},
		"offer": bson.M{
			"approved_amount": a.RequestedAmount,
			"interest_rate":   rate,
			"term_months":     a.TermMonths,
		},
		"status":     a.Status,
		"created_at": a.CreatedAt,
		"updated_at": a.UpdatedAt,
	}

	// Chi lien ket goi vay khi co ban ghi that.
	var packageRow struct {
		ID bson.ObjectID `bson:"_id"`
	}

	packageFilter := bson.M{
		"package_code": a.LoanPackageId,
	}

	if id, err := bson.ObjectIDFromHex(
		a.LoanPackageId,
	); err == nil {
		packageFilter = bson.M{"_id": id}
	}

	packageErr := m.db.Collection(
		"loan_packages",
	).FindOne(
		ctx,
		packageFilter,
	).Decode(&packageRow)

	if packageErr == nil {
		appDoc["loan_package_id"] = packageRow.ID
	} else if errors.Is(
		packageErr,
		mongodrv.ErrNoDocuments,
	) {
		// Catalog chua co: giu ma goc,
		// khong tao ObjectId gia.
		appDoc["loan_package_code"] = a.LoanPackageId
	} else {
		return packageErr
	}

	_, err = m.db.Collection(
		"loan_applications",
	).UpdateOne(
		ctx,
		bson.M{"_id": appOID},
		bson.M{"$setOnInsert": appDoc},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return err
	}

	// Khoan vay va lich EMI chinh thuc.
	loanDoc := bson.M{
		"loan_no":                 a.Id,
		"application_id":          appOID,
		"disbursement_account_id": accountID,
		"principal":               repayment.Principal,
		"outstanding_principal":   repayment.Principal,
		"interest_rate":           rate,
		"term_months":             repayment.TermMonths,
		"start_date":              repayment.DisbursedAt,
		"maturity_date":           repayment.Schedule[len(repayment.Schedule)-1].DueDate,
		"status":                  "active",
		"repayment_schedule": projectedInstallments(
			repayment.Schedule,
		),
		"created_at": repayment.DisbursedAt,
		"updated_at": repayment.DisbursedAt,
	}

	// SetOnInsert: replay LoanDisbursed khong
	// reset du no hoac cac ky da thanh toan.
	_, err = m.db.Collection("loans").UpdateOne(
		ctx,
		bson.M{"_id": loanOID},
		bson.M{"$setOnInsert": loanDoc},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return err
	}

	return m.ensureLoanContractFields(ctx, loanOID, loanDoc, repayment)
}

// Ap dung LoanInstallmentPaid.
// Ham nay PHAI duoc goi ben trong MongoDB
// transaction cua BankProjection.Apply.
func (p *BankProjection) applyLoanInstallmentPaid(
	ctx context.Context,
	record journal.Record,
	sequence int64,
) error {
	payment := record.LoanPayment

	if payment == nil {
		return fmt.Errorf("missing loan payment")
	}

	if err := payment.Validate(); err != nil {
		return err
	}

	if record.BalanceBefore == nil ||
		record.BalanceAfter == nil {
		return fmt.Errorf("missing loan debit balance")
	}

	loanOID := repaymentObjectID(
		"loan",
		payment.LoanID,
	)

	accountID, err := accountRecordID(
		payment.AccountID,
	)
	if err != nil {
		return err
	}

	var account struct {
		CustomerID bson.ObjectID `bson:"customer_id"`
		AccountNo  string        `bson:"account_no"`
		Username   string        `bson:"username"`
	}

	err = p.db.Collection("accounts").FindOne(
		ctx,
		bson.M{"_id": accountID},
	).Decode(&account)
	if err != nil {
		return fmt.Errorf(
			"repayment account missing: %w",
			err,
		)
	}

	var loanRow struct {
		LoanNo                string `bson:"loan_no"`
		DisbursementAccountID any    `bson:"disbursement_account_id"`
	}

	err = p.db.Collection("loans").FindOne(
		ctx,
		bson.M{"_id": loanOID},
	).Decode(&loanRow)
	if err != nil {
		return fmt.Errorf(
			"loan has not been projected: %w",
			err,
		)
	}

	// Source account may differ from the disbursement account but must
	// belong to the same customer profile. EventStoreDB checks this on write.
	if loanRow.DisbursementAccountID != accountID {
		var disbursement struct {
			CustomerID bson.ObjectID `bson:"customer_id"`
		}
		if err := p.db.Collection("accounts").FindOne(
			ctx, bson.M{"_id": loanRow.DisbursementAccountID},
		).Decode(&disbursement); err != nil || disbursement.CustomerID != account.CustomerID {
			return fmt.Errorf("repayment source is not owned by loan customer")
		}
	}

	newStatus := "active"
	if payment.OutstandingPrincipalAfter == 0 {
		newStatus = "completed"
	}

	// Chi cap nhat neu du no va ky thanh toan
	// dung voi lich EMI hien tai.
	paid := p.db.Collection("loan_payments")

	result, err := p.db.Collection("loans").UpdateOne(
		ctx,
		bson.M{
			"_id": loanOID,
			"outstanding_principal": payment.OutstandingPrincipalAfter +
				payment.PrincipalAmount,
			"repayment_schedule": bson.M{
				"$elemMatch": bson.M{
					"installment_no": payment.InstallmentNo,
					"due_date":       payment.DueDate,
					"principal_due":  payment.PrincipalAmount,
					"interest_due":   payment.InterestAmount,
					"total_due":      payment.Amount,
					"status":         "upcoming",
				},
			},
		},
		bson.M{"$set": bson.M{
			"outstanding_principal":       payment.OutstandingPrincipalAfter,
			"repayment_schedule.$.status": "paid",
			"status":                      newStatus,
			"updated_at":                  payment.PaidAt,
		}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		// Chi cho replay khi biên nhan trung
		// khop da duoc luu truoc do.
		var previous struct {
			TransactionID   string `bson:"transaction_id"`
			Amount          int64  `bson:"amount"`
			PrincipalAmount int64  `bson:"principal_amount"`
			InterestAmount  int64  `bson:"interest_amount"`
		}

		err = paid.FindOne(
			ctx,
			bson.M{
				"loan_id":        loanOID,
				"installment_no": payment.InstallmentNo,
			},
		).Decode(&previous)

		if err != nil ||
			previous.TransactionID != payment.TransactionID ||
			previous.Amount != payment.Amount ||
			previous.PrincipalAmount != payment.PrincipalAmount ||
			previous.InterestAmount != payment.InterestAmount {
			return fmt.Errorf(
				"loan repayment projection mismatch or loan not yet projected",
			)
		}
	}

	// Cap nhat so du tai khoan tu Banking Event.
	if err := p.balance(
		ctx,
		journal.FinancialAccountID(payment.AccountID),
		*record.BalanceAfter,
		sequence,
		record.At,
	); err != nil {
		return err
	}

	// Luu lich su thanh toan.
	_, err = paid.UpdateOne(
		ctx,
		bson.M{
			"_id": repaymentObjectID(
				"loan-payment",
				payment.PaymentID,
			),
		},
		bson.M{"$setOnInsert": bson.M{
			"loan_id":          loanOID,
			"customer_id":      account.CustomerID,
			"installment_no":   payment.InstallmentNo,
			"transaction_id":   payment.TransactionID,
			"amount":           payment.Amount,
			"principal_amount": payment.PrincipalAmount,
			"interest_amount":  payment.InterestAmount,
			"penalty_amount":   int64(0),
			"channel":          "bank_account",
			"status":           "success",
			"paid_at":          payment.PaidAt,
			"created_at":       payment.PaidAt,
		}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return err
	}

	// Giao dich tra no duoc lay tu Banking Event.
	// accountID la _id goc cua accounts:
	// ObjectId (24 hex) hoac string (32 hex).
	txRow := bson.M{
		"transaction_id": payment.TransactionID,
		"reference":      "LR" + payment.TransactionID,
		"type":           "loan_repayment",

		"source": bson.M{
			"account_id": accountID,
			"account_no": account.AccountNo,
			"name":       account.Username,
		},

		"destination": bson.M{
			"type":       "loan",
			"account_no": loanRow.LoanNo,
		},

		"amount":   payment.Amount,
		"fee":      int64(0),
		"currency": "VND",
		"channel":  "bank_account",
		"status":   "success",
		"note":     "Loan repayment",

		"idempotency_key": payment.TransactionID,

		"metadata": bson.M{
			"loan_id":        payment.LoanID,
			"installment_no": payment.InstallmentNo,
		},

		"created_at": payment.PaidAt,
		"posted_at":  payment.PaidAt,
	}

	_, err = p.db.Collection("transactions").UpdateOne(
		ctx,
		bson.M{
			"transaction_id": payment.TransactionID,
		},
		bson.M{"$setOnInsert": txRow},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return err
	}

	// Ledger dung financial_id co kieu ObjectId,
	// khong dung truc tiep _id string cua account cu.
	ledger := bson.M{
		"transaction_id": payment.TransactionID,
		"account_id": financialRecordID(
			payment.AccountID,
		),
		"sequence":       sequence,
		"direction":      "DEBIT",
		"amount":         payment.Amount,
		"balance_before": *record.BalanceBefore,
		"balance_after":  *record.BalanceAfter,
		"created_at":     payment.PaidAt,
	}

	_, err = p.db.Collection("ledger_entries").UpdateOne(
		ctx,
		bson.M{
			"transaction_id": payment.TransactionID,
			"direction":      "DEBIT",
		},
		bson.M{"$setOnInsert": ledger},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return err
	}

	// Feed su dung cung financial_id voi cac
	// giao dich Banking hien tai.
	feed := bson.M{
		"account_id": financialRecordID(
			payment.AccountID,
		),
		"transaction_id": payment.TransactionID,
		"direction":      "OUT",
		"type":           "loan_repayment",

		"counterparty": bson.M{
			"account_no": "",
			"name":       "ANOMALY LOAN",
			"bank_code":  "ANOMALY",
		},

		"amount":    payment.Amount,
		"fee":       int64(0),
		"status":    "success",
		"note":      "Loan repayment",
		"reference": "LR" + payment.TransactionID,
		"search_text": normalizeFeedSearch(
			"loan repayment " +
				loanRow.LoanNo + " " +
				payment.TransactionID,
		),

		"metadata": bson.M{
			"loan_id":        payment.LoanID,
			"installment_no": payment.InstallmentNo,
		},

		"balance_after": *record.BalanceAfter,
		"occurred_at":   payment.PaidAt,
	}

	_, err = p.db.Collection(
		"account_transaction_feed",
	).UpdateOne(
		ctx,

		bson.M{
			"account_id": financialRecordID(
				payment.AccountID,
			),
			"transaction_id": payment.TransactionID,
		},

		bson.M{"$setOnInsert": feed},
		options.UpdateOne().SetUpsert(true),
	)

	return err
}
