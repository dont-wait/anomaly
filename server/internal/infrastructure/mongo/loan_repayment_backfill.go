package mongo

import (
	"context"
	"fmt"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ensureLoanContractFields fills only absent contract fields. Payment receipts
// are checked before reconstructing missing EMI state; a partially repaid loan
// must never be reset to its original outstanding principal during backfill.
func (m *LoanReadModel) ensureLoanContractFields(
	ctx context.Context,
	loanOID bson.ObjectID,
	contract bson.M,
	repayment *loandomain.LoanRepayment,
) error {
	// A concurrent projector may create the same document. Re-read and retry
	// only when a guarded update detects that another writer changed it.
	for attempt := 0; attempt < 3; attempt++ {
		var existing struct {
			Principal   int64                     `bson:"principal"`
			TermMonths  int                       `bson:"term_months"`
			Outstanding *int64                    `bson:"outstanding_principal"`
			Status      string                    `bson:"status"`
			Schedule    []repaymentInstallmentRow `bson:"repayment_schedule"`
		}
		var raw bson.M
		if err := m.db.Collection("loans").FindOne(ctx, bson.M{"_id": loanOID}).Decode(&raw); err != nil {
			return err
		}
		encoded, err := bson.Marshal(raw)
		if err != nil {
			return err
		}
		if err := bson.Unmarshal(encoded, &existing); err != nil {
			return err
		}

		if _, ok := raw["principal"]; ok && existing.Principal != repayment.Principal {
			return fmt.Errorf("loan %s: existing principal differs from disbursement", repayment.LoanID)
		}
		if _, ok := raw["term_months"]; ok && existing.TermMonths != repayment.TermMonths {
			return fmt.Errorf("loan %s: existing term differs from disbursement", repayment.LoanID)
		}
		if old, ok := raw["application_id"]; ok && old != contract["application_id"] {
			return fmt.Errorf("loan %s: existing application reference differs", repayment.LoanID)
		}
		if old, ok := raw["disbursement_account_id"]; ok && old != contract["disbursement_account_id"] {
			return fmt.Errorf("loan %s: existing disbursement account differs", repayment.LoanID)
		}

		// Payment receipts are the read-side evidence of earlier installments.
		// They must match the immutable EMI schedule before filling legacy fields.
		cursor, err := m.db.Collection("loan_payments").Find(ctx,
			bson.M{"loan_id": loanOID},
			options.Find().SetSort(bson.D{{Key: "installment_no", Value: 1}}),
		)
		if err != nil {
			return err
		}
		paidCount := 0
		remaining := repayment.Principal
		for cursor.Next(ctx) {
			var payment struct {
				InstallmentNo int    `bson:"installment_no"`
				Principal     int64  `bson:"principal_amount"`
				Interest      int64  `bson:"interest_amount"`
				Amount        int64  `bson:"amount"`
				Status        string `bson:"status"`
			}
			if err := cursor.Decode(&payment); err != nil {
				_ = cursor.Close(ctx)
				return err
			}
			if paidCount >= len(repayment.Schedule) {
				_ = cursor.Close(ctx)
				return fmt.Errorf("loan %s: more payments than installments", repayment.LoanID)
			}
			item := repayment.Schedule[paidCount]
			if payment.InstallmentNo != paidCount+1 ||
				payment.Status != "success" ||
				payment.Principal != item.PrincipalAmount ||
				payment.Interest != item.InterestAmount ||
				payment.Amount != item.TotalPayment {
				_ = cursor.Close(ctx)
				return fmt.Errorf("loan %s: existing payment conflicts with signed EMI", repayment.LoanID)
			}
			remaining -= item.PrincipalAmount
			paidCount++
		}
		err = cursor.Err()
		_ = cursor.Close(ctx)
		if err != nil {
			return err
		}

		expectedStatus := "active"
		if paidCount == len(repayment.Schedule) {
			expectedStatus = "completed"
		}
		if existing.Outstanding != nil && *existing.Outstanding != remaining {
			return fmt.Errorf("loan %s: outstanding principal conflicts with %d recorded payments", repayment.LoanID, paidCount)
		}
		if existing.Status != "" && existing.Status != expectedStatus {
			return fmt.Errorf("loan %s: status conflicts with recorded payments", repayment.LoanID)
		}

		if len(existing.Schedule) > 0 {
			if len(existing.Schedule) != len(repayment.Schedule) {
				return fmt.Errorf("loan %s: incomplete existing EMI schedule", repayment.LoanID)
			}
			for i, item := range repayment.Schedule {
				stored := existing.Schedule[i]
				status := "upcoming"
				if i < paidCount {
					status = "paid"
				}
				if stored.InstallmentNo != item.InstallmentNo ||
					!stored.DueDate.Equal(item.DueDate) ||
					stored.PrincipalDue != item.PrincipalAmount ||
					stored.InterestDue != item.InterestAmount ||
					stored.TotalDue != item.TotalPayment ||
					stored.Status != status {
					return fmt.Errorf("loan %s: existing EMI schedule conflicts with recorded payments", repayment.LoanID)
				}
			}
		} else if _, ok := raw["repayment_schedule"]; ok {
			return fmt.Errorf("loan %s: existing EMI schedule is empty or invalid", repayment.LoanID)
		}

		toSet := bson.M{}
		for key, value := range contract {
			if _, present := raw[key]; !present {
				toSet[key] = value
			}
		}
		if _, exists := raw["outstanding_principal"]; !exists {
			toSet["outstanding_principal"] = remaining
		}
		if _, exists := raw["status"]; !exists {
			toSet["status"] = expectedStatus
		}
		if _, exists := raw["repayment_schedule"]; !exists {
			items := projectedInstallments(repayment.Schedule)
			for i := 0; i < paidCount; i++ {
				items[i]["status"] = "paid"
			}
			toSet["repayment_schedule"] = items
		}
		if len(toSet) == 0 {
			return nil
		}

		filter := bson.M{"_id": loanOID}
		for key := range toSet {
			filter[key] = bson.M{"$exists": false}
		}
		result, err := m.db.Collection("loans").UpdateOne(ctx, filter, bson.M{"$set": toSet})
		if err != nil {
			return err
		}
		if result.MatchedCount == 1 {
			// Verify the document after the guarded update, rather than counting
			// an existing unchanged loan as a successful backfill.
			continue
		}
	}
	return fmt.Errorf("loan %s: backfill could not verify stable repayment fields", repayment.LoanID)
}
