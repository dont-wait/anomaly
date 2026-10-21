package mongo

import (
	"context"
	"errors"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

// LoanContractQuery reads MongoDB projections only. The ownership gate is the
// EventStoreDB-backed loan_applications_read projection, not the ObjectID in loans.
type LoanContractQuery struct {
	db *mongodrv.Database
}

func NewLoanContractQuery(read *LoanReadModel) *LoanContractQuery {
	return &LoanContractQuery{db: read.db}
}

func validProjectedContract(row repaymentLoanRow, applicationID string) bool {
	_, expectedApplicationID := loanProjectionIDs(applicationID)
	if row.ApplicationID != expectedApplicationID || row.LoanNo != applicationID || row.Principal <= 0 ||
		row.OutstandingPrincipal < 0 || row.OutstandingPrincipal > row.Principal ||
		row.TermMonths < 1 || row.StartDate.IsZero() || row.MaturityDate.IsZero() ||
		len(row.Schedule) != row.TermMonths ||
		(row.Status != "active" && row.Status != "completed") {
		return false
	}
	if row.Status == "completed" && row.OutstandingPrincipal != 0 {
		return false
	}
	paid, totalPrincipal, paidPrincipal := 0, int64(0), int64(0)
	for i, item := range row.Schedule {
		if item.InstallmentNo != i+1 || item.DueDate.IsZero() ||
			item.PrincipalDue < 0 || item.InterestDue < 0 ||
			item.TotalDue != item.PrincipalDue+item.InterestDue ||
			(item.Status != "paid" && item.Status != "upcoming") {
			return false
		}
		if item.Status == "paid" {
			paid++
			paidPrincipal += item.PrincipalDue
		}
		totalPrincipal += item.PrincipalDue
	}
	if totalPrincipal != row.Principal || row.OutstandingPrincipal != totalPrincipal-paidPrincipal ||
		(row.Status == "completed" && paid != len(row.Schedule)) {
		return false
	}
	return true
}

func contractSummary(application loandomain.Application, row repaymentLoanRow) loanqueries.ContractSummary {
	schedule := mapSchedule(application.Id, row)
	paid := 0
	var nextDate string
	var nextAmount int64
	for _, item := range schedule.Items {
		if item.Status == "paid" {
			paid++
		}
		if item.InstallmentNo == schedule.NextInstallmentNo {
			nextDate = item.DueDate
			nextAmount = item.TotalDue
		}
	}
	return loanqueries.ContractSummary{
		LoanID:               application.Id,
		LoanNo:               row.LoanNo,
		ApplicationID:        application.Id,
		LoanPackageID:        application.LoanPackageId,
		Principal:            row.Principal,
		OutstandingPrincipal: row.OutstandingPrincipal,
		AnnualInterestRate:   row.InterestRate.String(),
		TermMonths:           row.TermMonths,
		Status:               row.Status,
		DisbursedAt:          row.StartDate,
		MaturityDate:         row.MaturityDate,
		PaidInstallments:     paid,
		TotalInstallments:    len(schedule.Items),
		NextInstallmentNo:    schedule.NextInstallmentNo,
		NextDueDate:          nextDate,
		NextDueAmount:        nextAmount,
	}
}

// ListContractPage pages over disbursed applications, then batch-loads the
// corresponding contracts. A missing loan row is not silently omitted: this
// signals projection lag or inconsistency so clients can retry later.
func (r *LoanContractQuery) ListContractPage(ctx context.Context, customerID string, limit int, afterTime time.Time, afterID string) ([]loanqueries.ContractListRow, bool, error) {
	if r == nil || r.db == nil || customerID == "" || limit < 1 || limit > 50 {
		return nil, false, loanqueries.ErrInvalidLoanContractQuery
	}

	read := &LoanReadModel{db: r.db}
	apps, hasMore, err := read.listPage(ctx, bson.M{
		"owner_id": customerID,
		"status":   string(loandomain.ApplicationStatusDisbursed),
	}, limit, afterTime, afterID)
	if err != nil {
		return nil, false, err
	}
	if len(apps) == 0 {
		return []loanqueries.ContractListRow{}, false, nil
	}

	ids := make(bson.A, 0, len(apps))
	for _, app := range apps {
		if app.Id == "" || app.CustomerId != customerID || app.Status != loandomain.ApplicationStatusDisbursed {
			return nil, false, loanqueries.ErrLoanContractNotReady
		}
		loanOID, _ := loanProjectionIDs(app.Id)
		ids = append(ids, loanOID)
	}

	cursor, err := r.db.Collection("loans").Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, false, err
	}
	defer cursor.Close(ctx)

	byID := make(map[bson.ObjectID]repaymentLoanRow, len(apps))
	for cursor.Next(ctx) {
		var row repaymentLoanRow
		if err := cursor.Decode(&row); err != nil {
			return nil, false, err
		}
		byID[row.ID] = row
	}
	if err := cursor.Err(); err != nil {
		return nil, false, err
	}

	items := make([]loanqueries.ContractListRow, 0, len(apps))
	for _, app := range apps {
		loanOID, _ := loanProjectionIDs(app.Id)
		row, ok := byID[loanOID]
		if !ok || !validProjectedContract(row, app.Id) {
			return nil, false, loanqueries.ErrLoanContractNotReady
		}
		items = append(items, loanqueries.ContractListRow{
			Contract:             contractSummary(*app, row),
			ApplicationCreatedAt: app.CreatedAt,
			ApplicationID:        app.Id,
		})
	}
	return items, hasMore, nil
}

func (r *LoanContractQuery) ReadContract(ctx context.Context, loanID, customerID string) (loanqueries.ContractDetail, error) {
	if r == nil || r.db == nil {
		return loanqueries.ContractDetail{}, loanqueries.ErrInvalidLoanContractQuery
	}
	var appRecord struct {
		Application loandomain.Application `bson:"application"`
	}
	err := r.db.Collection("loan_applications_read").FindOne(ctx, bson.M{
		"_id":      loanID,
		"owner_id": customerID,
		"status":   string(loandomain.ApplicationStatusDisbursed),
	}).Decode(&appRecord)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return loanqueries.ContractDetail{}, loanqueries.ErrLoanContractNotFound
	}
	if err != nil {
		return loanqueries.ContractDetail{}, err
	}
	if appRecord.Application.Id != loanID || appRecord.Application.CustomerId != customerID ||
		appRecord.Application.Status != loandomain.ApplicationStatusDisbursed {
		return loanqueries.ContractDetail{}, loanqueries.ErrLoanContractNotReady
	}

	loanOID, appOID := loanProjectionIDs(loanID)
	var row repaymentLoanRow
	err = r.db.Collection("loans").FindOne(ctx, bson.M{
		"_id": loanOID, "application_id": appOID,
	}).Decode(&row)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return loanqueries.ContractDetail{}, loanqueries.ErrLoanContractNotReady
	}
	if err != nil {
		return loanqueries.ContractDetail{}, err
	}
	if !validProjectedContract(row, loanID) {
		return loanqueries.ContractDetail{}, loanqueries.ErrLoanContractNotReady
	}
	schedule := mapSchedule(loanID, row)
	summary := contractSummary(appRecord.Application, row)
	return loanqueries.ContractDetail{
		LoanID:               summary.LoanID,
		LoanNo:               summary.LoanNo,
		ApplicationID:        summary.ApplicationID,
		LoanPackageID:        summary.LoanPackageID,
		Principal:            summary.Principal,
		OutstandingPrincipal: summary.OutstandingPrincipal,
		AnnualInterestRate:   summary.AnnualInterestRate,
		TermMonths:           summary.TermMonths,
		Status:               summary.Status,
		DisbursedAt:          summary.DisbursedAt,
		MaturityDate:         summary.MaturityDate,
		PaidInstallments:     summary.PaidInstallments,
		TotalInstallments:    summary.TotalInstallments,
		NextInstallmentNo:    summary.NextInstallmentNo,
		NextDueDate:          summary.NextDueDate,
		NextDueAmount:        summary.NextDueAmount,
		RepaymentMethod:      "fixed_total_reducing_balance",
		Schedule:             schedule.Items,
	}, nil
}
