package mongo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// LoanContractQuery reads MongoDB projections only. The ownership gate is the
// EventStoreDB-backed loan_applications_read projection, not the ObjectID in loans.
type LoanContractQuery struct {
	db *mongodrv.Database
}

func NewLoanContractQuery(read *LoanReadModel) *LoanContractQuery {
	return &LoanContractQuery{db: read.db}
}

func (r *LoanContractQuery) projectionRebuilding(
	ctx context.Context,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, loanqueries.ErrInvalidLoanContractQuery
	}

	read := &LoanReadModel{db: r.db}
	return read.LoanProjectionRebuilding(ctx)
}

func (r *LoanContractQuery) ContractSnapshot(
	ctx context.Context,
	customerID string,
) (string, error) {
	if r == nil || r.db == nil || customerID == "" {
		return "", loanqueries.ErrInvalidLoanContractQuery
	}

	rebuilding, err := r.projectionRebuilding(ctx)
	if err != nil {
		return "", err
	}
	if rebuilding {
		return "", loanqueries.ErrLoanContractNotReady
	}

	cursor, err := r.db.Collection("loan_applications_read").Find(
		ctx,
		bson.M{
			"owner_id": customerID,
			"status":   string(loandomain.ApplicationStatusDisbursed),
		},
		options.Find().
			SetProjection(bson.M{
				"_id":        1,
				"created_at": 1,
				"revision":   1,
			}).
			SetSort(bson.D{{Key: "_id", Value: 1}}),
	)
	if err != nil {
		return "", err
	}
	defer cursor.Close(ctx)

	hash := sha256.New()

	for cursor.Next(ctx) {
		var record struct {
			ID        string    `bson:"_id"`
			CreatedAt time.Time `bson:"created_at"`
			Revision  uint64    `bson:"revision"`
		}

		if err := cursor.Decode(&record); err != nil {
			return "", err
		}

		if record.ID == "" || record.CreatedAt.IsZero() {
			return "", loanqueries.ErrLoanContractNotReady
		}

		_, _ = fmt.Fprintf(
			hash,
			"%d:%s|%d|%d\n",
			len(record.ID),
			record.ID,
			record.CreatedAt.UnixMilli(),
			record.Revision,
		)
	}

	if err := cursor.Err(); err != nil {
		return "", err
	}

	rebuilding, err = r.projectionRebuilding(ctx)
	if err != nil {
		return "", err
	}
	if rebuilding {
		return "", loanqueries.ErrLoanContractNotReady
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validProjectedContract(
	row repaymentLoanRow,
	application loandomain.Application,
) bool {
	applicationID := application.Id

	if applicationID == "" ||
		application.Status != loandomain.ApplicationStatusDisbursed ||
		application.RequestedAmount <= 0 ||
		application.TermMonths <= 0 {
		return false
	}

	_, expectedApplicationID := loanProjectionIDs(applicationID)

	if row.ApplicationID != expectedApplicationID ||
		row.LoanNo != applicationID ||
		row.Principal <= 0 ||
		row.Principal != application.RequestedAmount ||
		row.TermMonths != application.TermMonths ||
		row.OutstandingPrincipal < 0 ||
		row.OutstandingPrincipal > row.Principal ||
		row.TermMonths < 1 ||
		row.StartDate.IsZero() ||
		row.MaturityDate.IsZero() ||
		!row.MaturityDate.After(row.StartDate) ||
		len(row.Schedule) != row.TermMonths ||
		(row.Status != "active" && row.Status != "completed") {
		return false
	}

	// The signed rate must be a finite, positive decimal.
	rate, _, err := big.ParseFloat(row.InterestRate.String(), 10, 128, big.ToNearestEven)
	if err != nil || rate == nil || rate.IsInf() || rate.Sign() <= 0 {
		return false
	}

	paid := 0
	var totalPrincipal int64
	var paidPrincipal int64
	previousDueDate := row.StartDate

	for i, item := range row.Schedule {
		if item.InstallmentNo != i+1 ||
			item.DueDate.IsZero() ||
			!item.DueDate.After(previousDueDate) ||
			item.PrincipalDue < 0 ||
			item.InterestDue < 0 ||
			(item.Status != "paid" && item.Status != "upcoming") {
			return false
		}

		// Prevent integer overflow and inconsistent installment amounts.
		if item.InterestDue > int64(^uint64(0)>>1)-item.PrincipalDue {
			return false
		}

		if item.TotalDue != item.PrincipalDue+item.InterestDue {
			return false
		}

		if item.PrincipalDue > row.Principal-totalPrincipal {
			return false
		}

		totalPrincipal += item.PrincipalDue

		if item.Status == "paid" {
			paid++
			paidPrincipal += item.PrincipalDue
		}

		previousDueDate = item.DueDate
	}

	// The final installment date must match contract maturity.
	if !previousDueDate.Equal(row.MaturityDate) {
		return false
	}

	if totalPrincipal != row.Principal ||
		row.OutstandingPrincipal != totalPrincipal-paidPrincipal {
		return false
	}

	// An active loan must still have outstanding debt
	// and at least one unpaid installment.
	if row.Status == "active" {
		if row.OutstandingPrincipal == 0 ||
			paid == len(row.Schedule) {
			return false
		}
	}

	// A completed loan must have no outstanding principal
	// and every installment must be paid.
	if row.Status == "completed" {
		if row.OutstandingPrincipal != 0 ||
			paid != len(row.Schedule) {
			return false
		}
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

type contractApplicationRecord struct {
	ID          string                 `bson:"_id"`
	OwnerID     string                 `bson:"owner_id"`
	CreatedAt   time.Time              `bson:"created_at"`
	Status      string                 `bson:"status"`
	Application loandomain.Application `bson:"application"`
}

func (r *LoanContractQuery) listContractApplications(
	ctx context.Context,
	customerID string,
	limit int,
	afterTime time.Time,
	afterID string,
) ([]contractApplicationRecord, bool, error) {
	filter := bson.M{
		"owner_id": customerID,
		"status": string(
			loandomain.ApplicationStatusDisbursed,
		),
	}

	if afterID != "" {
		if afterTime.IsZero() {
			return nil, false,
				loanqueries.ErrInvalidLoanContractQuery
		}

		filter["$or"] = bson.A{
			bson.M{
				"created_at": bson.M{"$lt": afterTime},
			},
			bson.M{
				"created_at": afterTime,
				"_id":        bson.M{"$lt": afterID},
			},
		}
	}

	cursor, err := r.db.Collection(
		"loan_applications_read",
	).Find(
		ctx,
		filter,
		options.Find().
			SetSort(bson.D{
				{Key: "created_at", Value: -1},
				{Key: "_id", Value: -1},
			}).
			SetLimit(int64(limit+1)),
	)
	if err != nil {
		return nil, false, err
	}
	defer cursor.Close(ctx)

	records := make(
		[]contractApplicationRecord,
		0,
		limit,
	)

	hasMore := false

	for cursor.Next(ctx) {
		if len(records) == limit {
			hasMore = true
			break
		}

		var record contractApplicationRecord

		if err := cursor.Decode(&record); err != nil {
			return nil, false, err
		}

		// The embedded application must match the
		// authoritative MongoDB document identity.
		if record.ID == "" ||
			record.ID != record.Application.Id ||
			record.OwnerID != customerID ||
			record.Application.CustomerId != customerID ||
			record.Status != string(
				loandomain.ApplicationStatusDisbursed,
			) ||
			record.Application.Status !=
				loandomain.ApplicationStatusDisbursed {
			return nil, false,
				loanqueries.ErrLoanContractNotReady
		}

		// Cursor timestamps must match the actual
		// database sort key.
		if record.CreatedAt.IsZero() ||
			record.Application.CreatedAt.IsZero() ||
			!record.CreatedAt.Equal(
				record.Application.CreatedAt,
			) {
			return nil, false,
				loanqueries.ErrLoanContractNotReady
		}

		records = append(records, record)
	}

	if err := cursor.Err(); err != nil {
		return nil, false, err
	}

	return records, hasMore, nil
}

func (r *LoanContractQuery) ListContractPage(
	ctx context.Context,
	customerID string,
	limit int,
	afterTime time.Time,
	afterID string,
) ([]loanqueries.ContractListRow, bool, error) {
	if r == nil || r.db == nil ||
		customerID == "" ||
		limit < 1 || limit > 50 {
		return nil, false,
			loanqueries.ErrInvalidLoanContractQuery
	}

	rebuilding, err := r.projectionRebuilding(ctx)
	if err != nil {
		return nil, false, err
	}
	if rebuilding {
		return nil, false, loanqueries.ErrLoanContractNotReady
	}

	apps, hasMore, err := r.listContractApplications(
		ctx,
		customerID,
		limit,
		afterTime,
		afterID,
	)
	if err != nil {
		return nil, false, err
	}

	if len(apps) == 0 {
		rebuilding, checkErr := r.projectionRebuilding(ctx)
		if checkErr != nil {
			return nil, false, checkErr
		}
		if rebuilding {
			return nil, false, loanqueries.ErrLoanContractNotReady
		}

		return []loanqueries.ContractListRow{}, false, nil
	}

	ids := make(bson.A, 0, len(apps))

	for _, record := range apps {
		loanOID, _ := loanProjectionIDs(record.ID)
		ids = append(ids, loanOID)
	}

	cursor, err := r.db.Collection("loans").Find(
		ctx,
		bson.M{
			"_id": bson.M{"$in": ids},
		},
	)
	if err != nil {
		return nil, false, err
	}
	defer cursor.Close(ctx)

	byID := make(
		map[bson.ObjectID]repaymentLoanRow,
		len(apps),
	)

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

	items := make(
		[]loanqueries.ContractListRow,
		0,
		len(apps),
	)

	for _, record := range apps {
		loanOID, _ := loanProjectionIDs(record.ID)

		row, ok := byID[loanOID]
		if !ok ||
			!validProjectedContract(
				row,
				record.Application,
			) {
			return nil, false,
				loanqueries.ErrLoanContractNotReady
		}

		items = append(
			items,
			loanqueries.ContractListRow{
				Contract: contractSummary(
					record.Application,
					row,
				),
				ApplicationCreatedAt: record.CreatedAt,
				ApplicationID:        record.ID,
			},
		)
	}

	rebuilding, err = r.projectionRebuilding(ctx)
	if err != nil {
		return nil, false, err
	}

	if rebuilding {
		return nil, false, loanqueries.ErrLoanContractNotReady
	}

	return items, hasMore, nil

}

func (r *LoanContractQuery) ReadContract(
	ctx context.Context,
	loanID, customerID string,
) (loanqueries.ContractDetail, error) {
	if r == nil || r.db == nil {
		return loanqueries.ContractDetail{},
			loanqueries.ErrInvalidLoanContractQuery
	}

	rebuilding, err := r.projectionRebuilding(ctx)
	if err != nil {
		return loanqueries.ContractDetail{}, err
	}

	if rebuilding {
		return loanqueries.ContractDetail{},
			loanqueries.ErrLoanContractNotReady
	}

	var appRecord struct {
		Application loandomain.Application `bson:"application"`
	}
	err = r.db.Collection("loan_applications_read").FindOne(ctx, bson.M{
		"_id":      loanID,
		"owner_id": customerID,
		"status":   string(loandomain.ApplicationStatusDisbursed),
	}).Decode(&appRecord)

	if errors.Is(err, mongodrv.ErrNoDocuments) {
		rebuilding, checkErr := r.projectionRebuilding(ctx)
		if checkErr != nil {
			return loanqueries.ContractDetail{}, checkErr
		}
		if rebuilding {
			return loanqueries.ContractDetail{},
				loanqueries.ErrLoanContractNotReady
		}

		return loanqueries.ContractDetail{},
			loanqueries.ErrLoanContractNotFound
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
	if !validProjectedContract(row, appRecord.Application) {
		return loanqueries.ContractDetail{}, loanqueries.ErrLoanContractNotReady
	}

	rebuilding, err = r.projectionRebuilding(ctx)
	if err != nil {
		return loanqueries.ContractDetail{}, err
	}

	if rebuilding {
		return loanqueries.ContractDetail{},
			loanqueries.ErrLoanContractNotReady
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
