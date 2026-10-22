package mongo

import (
	"context"
	"errors"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/timeutil"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type LoanRepaymentQuery struct {
	db *mongodrv.Database
}

func NewLoanRepaymentQuery(
	client *mongodrv.Client,
	dbName string,
) *LoanRepaymentQuery {
	return &LoanRepaymentQuery{
		db: client.Database(dbName),
	}
}

// Kiem tra khoan vay co thuoc khach hang khong.
//
// EventStoreDB luu LoanID dang string.
// MongoDB loans._id su dung ObjectId on dinh
// da tao o Giai doan 3.
func (r *LoanRepaymentQuery) ownedLoan(
	ctx context.Context,
	loanID, customerID string,
) (bson.ObjectID, error) {
	if loanID == "" || customerID == "" {
		return bson.ObjectID{},
			loanqueries.ErrLoanNotFound
	}

	// Xac nhan quyen so huu tu Read Model.
	err := r.db.Collection(
		"loan_applications_read",
	).FindOne(
		ctx,
		bson.M{
			"_id":      loanID,
			"owner_id": customerID,
			"status":   "disbursed",
		},
	).Err()

	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return bson.ObjectID{},
			loanqueries.ErrLoanNotFound
	}
	if err != nil {
		return bson.ObjectID{}, err
	}

	loanOID := repaymentObjectID(
		"loan", loanID,
	)

	appOID := repaymentObjectID(
		"loan-application", loanID,
	)

	err = r.db.Collection("loans").FindOne(
		ctx,
		bson.M{
			"_id":            loanOID,
			"application_id": appOID,
		},
	).Err()

	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return bson.ObjectID{},
			loanqueries.ErrLoanNotFound
	}
	if err != nil {
		return bson.ObjectID{}, err
	}

	return loanOID, nil
}

// Cac truong dang snake_case trong MongoDB.
type repaymentInstallmentRow struct {
	InstallmentNo int       `bson:"installment_no"`
	DueDate       time.Time `bson:"due_date"`
	PrincipalDue  int64     `bson:"principal_due"`
	InterestDue   int64     `bson:"interest_due"`
	TotalDue      int64     `bson:"total_due"`
	Status        string    `bson:"status"`
}

type repaymentLoanRow struct {
	ID bson.ObjectID `bson:"_id"`

	LoanNo               string          `bson:"loan_no"`
	Principal            int64           `bson:"principal"`
	OutstandingPrincipal int64           `bson:"outstanding_principal"`
	InterestRate         bson.Decimal128 `bson:"interest_rate"`
	TermMonths           int             `bson:"term_months"`
	Status               string          `bson:"status"`

	Schedule []repaymentInstallmentRow `bson:"repayment_schedule"`

	DisbursementAccountID any `bson:"disbursement_account_id"`
}

// Chuyen MongoDB document sang Query DTO.
func mapSchedule(
	loanID string,
	row repaymentLoanRow,
) loanqueries.Schedule {
	items := make(
		[]loanqueries.Installment,
		0,
		len(row.Schedule),
	)

	next := 0

	today := time.Now().In(
		timeutil.VietnamLocation,
	).Format("2006-01-02")

	for _, item := range row.Schedule {
		status := item.Status

		due := item.DueDate.UTC().Format(
			"2006-01-02",
		)

		if next == 0 && status != "paid" {
			next = item.InstallmentNo
		}

		if status == "upcoming" && due == today {
			status = "due"
		}

		items = append(
			items,
			loanqueries.Installment{
				InstallmentNo: item.InstallmentNo,
				DueDate:       due,
				PrincipalDue:  item.PrincipalDue,
				InterestDue:   item.InterestDue,
				TotalDue:      item.TotalDue,
				Status:        status,
			},
		)
	}

	return loanqueries.Schedule{
		LoanID:               loanID,
		LoanNo:               row.LoanNo,
		Principal:            row.Principal,
		OutstandingPrincipal: row.OutstandingPrincipal,
		AnnualInterestRate:   row.InterestRate.String(),
		TermMonths:           row.TermMonths,
		RepaymentMethod:      "fixed_total_reducing_balance",
		Status:               row.Status,
		Items:                items,
		NextInstallmentNo:    next,
	}
}

// Doc lich tra no tu collection loans.
func (r *LoanRepaymentQuery) ReadSchedule(
	ctx context.Context,
	loanID, customerID string,
) (loanqueries.Schedule, error) {
	oid, err := r.ownedLoan(
		ctx, loanID, customerID,
	)
	if err != nil {
		return loanqueries.Schedule{}, err
	}

	var row repaymentLoanRow

	err = r.db.Collection("loans").FindOne(
		ctx,
		bson.M{"_id": oid},
	).Decode(&row)

	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return loanqueries.Schedule{},
			loanqueries.ErrLoanNotFound
	}
	if err != nil {
		return loanqueries.Schedule{}, err
	}

	return mapSchedule(loanID, row), nil
}

// Cau truc mot thanh toan trong loan_payments.
type repaymentPaymentRow struct {
	ID              bson.ObjectID `bson:"_id"`
	TransactionID   string        `bson:"transaction_id"`
	InstallmentNo   int           `bson:"installment_no"`
	Amount          int64         `bson:"amount"`
	PrincipalAmount int64         `bson:"principal_amount"`
	InterestAmount  int64         `bson:"interest_amount"`
	PenaltyAmount   int64         `bson:"penalty_amount"`
	Channel         string        `bson:"channel"`
	Status          string        `bson:"status"`
	PaidAt          time.Time     `bson:"paid_at"`
}

func mapPayment(
	loanID string,
	row repaymentPaymentRow,
) loanqueries.Payment {
	return loanqueries.Payment{
		PaymentID:       row.ID.Hex(),
		LoanID:          loanID,
		TransactionID:   row.TransactionID,
		InstallmentNo:   row.InstallmentNo,
		Amount:          row.Amount,
		PrincipalAmount: row.PrincipalAmount,
		InterestAmount:  row.InterestAmount,
		PenaltyAmount:   row.PenaltyAmount,
		Channel:         row.Channel,
		Status:          row.Status,
		PaidAt:          row.PaidAt,
	}
}

// Danh sach cac lan thanh toan.
// Ban MVP ho tro toi da 60 ky.
func (r *LoanRepaymentQuery) ReadPayments(
	ctx context.Context,
	loanID, customerID string,
) ([]loanqueries.Payment, error) {
	oid, err := r.ownedLoan(
		ctx, loanID, customerID,
	)
	if err != nil {
		return nil, err
	}

	cursor, err := r.db.Collection(
		"loan_payments",
	).Find(
		ctx,
		bson.M{"loan_id": oid},
		options.Find().
			SetSort(
				bson.D{
					{Key: "installment_no", Value: -1},
				},
			).
			SetLimit(60),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	result := make([]loanqueries.Payment, 0)

	for cursor.Next(ctx) {
		var row repaymentPaymentRow

		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}

		result = append(
			result,
			mapPayment(loanID, row),
		)
	}

	return result, cursor.Err()
}

// Xem chi tiet mot lan thanh toan.
func (r *LoanRepaymentQuery) ReadPayment(
	ctx context.Context,
	loanID, customerID, paymentID string,
) (loanqueries.Payment, error) {
	oid, err := r.ownedLoan(
		ctx, loanID, customerID,
	)
	if err != nil {
		return loanqueries.Payment{}, err
	}

	payOID, err := bson.ObjectIDFromHex(
		paymentID,
	)
	if err != nil {
		return loanqueries.Payment{},
			loanqueries.ErrPaymentNotFound
	}

	var row repaymentPaymentRow

	err = r.db.Collection("loan_payments").FindOne(
		ctx,
		bson.M{
			"_id":     payOID,
			"loan_id": oid,
		},
	).Decode(&row)

	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return loanqueries.Payment{},
			loanqueries.ErrPaymentNotFound
	}
	if err != nil {
		return loanqueries.Payment{}, err
	}

	return mapPayment(loanID, row), nil
}

// Lay so du trong MongoDB Read Model.
//
// So du nay CHI dung de hien thi/tao Quote.
// Khi thu no, Banking Stream van kiem tra lai.
func (r *LoanRepaymentQuery) ReadAvailableBalance(
	ctx context.Context,
	loanID, customerID, accountID string,
) (int64, error) {
	oid, err := r.ownedLoan(
		ctx, loanID, customerID,
	)
	if err != nil {
		return 0, err
	}

	actorID, err := accountRecordID(customerID)
	if err != nil {
		return 0, loanqueries.ErrLoanNotFound
	}

	recordID, err := accountRecordID(accountID)
	if err != nil {
		return 0, loanqueries.ErrLoanNotFound
	}

	var loanRow repaymentLoanRow

	err = r.db.Collection("loans").FindOne(
		ctx,
		bson.M{"_id": oid},
	).Decode(&loanRow)
	if err != nil {
		return 0, err
	}

	// Disbursement must belong to the same customer profile as the actor.
	owner := struct {
		CustomerID bson.ObjectID `bson:"customer_id"`
	}{}
	if err := r.db.Collection("accounts").FindOne(
		ctx, bson.M{"_id": actorID, "status": "active"},
	).Decode(&owner); err != nil {
		if errors.Is(err, mongodrv.ErrNoDocuments) {
			return 0, loanqueries.ErrLoanNotFound
		}
		return 0, err
	}

	if loanRow.DisbursementAccountID != actorID {
		var disbursement struct {
			CustomerID bson.ObjectID `bson:"customer_id"`
		}
		err = r.db.Collection("accounts").FindOne(
			ctx, bson.M{"_id": loanRow.DisbursementAccountID},
		).Decode(&disbursement)
		if err != nil || disbursement.CustomerID != owner.CustomerID {
			return 0, loanqueries.ErrLoanNotFound
		}
	}

	var account struct {
		CustomerID bson.ObjectID `bson:"customer_id"`
		Balance    struct {
			Current int64 `bson:"current"`
		} `bson:"balance"`
	}

	err = r.db.Collection("accounts").FindOne(
		ctx,
		bson.M{
			"_id":         recordID,
			"customer_id": owner.CustomerID,
			"status":      "active",
			"type":        "payment",
			"currency":    "VND",
		},
	).Decode(&account)

	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return 0, loanqueries.ErrInvalidPaymentAccount
	}
	if err != nil {
		return 0, err
	}

	// Banking Write also requires verified KYC. A quote must not be
	// created for an account that would be rejected at debit time.
	err = r.db.Collection("customers").FindOne(ctx, bson.M{
		"_id":        owner.CustomerID,
		"kyc_status": "verified",
	}).Err()
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return 0, loanqueries.ErrInvalidPaymentAccount
	}
	if err != nil {
		return 0, err
	}

	return account.Balance.Current, nil
}
