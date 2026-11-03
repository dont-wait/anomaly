package mongo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// AdminLoanContractFilter lọc khoản vay của mọi khách.
type AdminLoanContractFilter struct {
	Status      string
	PackageCode string
	CustomerID  string
	Overdue     bool
}

type AdminLoanContract struct {
	loanqueries.ContractSummary
	CustomerID          string `json:"customerId"`
	OverdueInstallments int    `json:"overdueInstallments"`
	OldestOverdueDate   string `json:"oldestOverdueDate,omitempty"`
}

type AdminLoanContractPage struct {
	Items               []AdminLoanContract `json:"items"`
	NextCursor          *string             `json:"nextCursor"`
	EventualConsistency bool                `json:"eventualConsistency"`
}

type AdminLoanContractDetail struct {
	loanqueries.ContractDetail
	CustomerID string                `json:"customerId"`
	Payments   []loanqueries.Payment `json:"payments"`
}

var ErrInvalidAdminLoanCursor = errors.New("invalid loan contract cursor")

const adminLoanScanBatch = 100

type adminLoanCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

// AdminLoanContracts ghép hồ sơ đã giải ngân với khoản vay. Trạng thái quá hạn
// được tính lúc đọc nên lọc sau khi ghép, quét từng lô cho đến khi đủ trang.
type AdminLoanContracts struct {
	db        *mongodrv.Database
	contracts *LoanContractQuery
	payments  *LoanRepaymentQuery
}

func NewAdminLoanContracts(read *LoanReadModel, payments *LoanRepaymentQuery) *AdminLoanContracts {
	return &AdminLoanContracts{db: read.db, contracts: NewLoanContractQuery(read), payments: payments}
}

func overdueOf(row repaymentLoanRow, loanID string) (int, string) {
	count, oldest := 0, ""
	for _, item := range mapSchedule(loanID, row).Items {
		if item.Status == "overdue" {
			if count == 0 {
				oldest = item.DueDate
			}
			count++
		}
	}
	return count, oldest
}

func (q *AdminLoanContracts) List(ctx context.Context, f AdminLoanContractFilter, limit int, token string) (AdminLoanContractPage, error) {
	if limit < 1 || limit > 50 {
		return AdminLoanContractPage{}, ErrInvalidAdminLoanCursor
	}
	var after adminLoanCursor
	if token != "" {
		data, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || json.Unmarshal(data, &after) != nil || after.ID == "" {
			return AdminLoanContractPage{}, ErrInvalidAdminLoanCursor
		}
	}
	page := AdminLoanContractPage{Items: []AdminLoanContract{}, EventualConsistency: true}
	var lastReturned adminLoanCursor
	for {
		filter := bson.M{"status": string(loandomain.ApplicationStatusDisbursed)}
		if f.CustomerID != "" {
			filter["owner_id"] = f.CustomerID
		}
		if f.PackageCode != "" {
			filter["application.loanpackageid"] = f.PackageCode
		}
		if after.ID != "" {
			filter["$or"] = bson.A{
				bson.M{"created_at": bson.M{"$lt": after.CreatedAt}},
				bson.M{"created_at": after.CreatedAt, "_id": bson.M{"$lt": after.ID}},
			}
		}
		cur, err := q.db.Collection("loan_applications_read").Find(ctx, filter, options.Find().
			SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(adminLoanScanBatch))
		if err != nil {
			return AdminLoanContractPage{}, err
		}
		var apps []contractApplicationRecord
		if err := cur.All(ctx, &apps); err != nil {
			return AdminLoanContractPage{}, err
		}
		if len(apps) == 0 {
			return page, nil
		}
		ids := make(bson.A, 0, len(apps))
		for _, app := range apps {
			loanOID, _ := loanProjectionIDs(app.ID)
			ids = append(ids, loanOID)
		}
		loanCursor, err := q.db.Collection("loans").Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
		if err != nil {
			return AdminLoanContractPage{}, err
		}
		var rows []repaymentLoanRow
		if err := loanCursor.All(ctx, &rows); err != nil {
			return AdminLoanContractPage{}, err
		}
		byID := make(map[bson.ObjectID]repaymentLoanRow, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}
		for _, app := range apps {
			after = adminLoanCursor{CreatedAt: app.CreatedAt, ID: app.ID}
			loanOID, _ := loanProjectionIDs(app.ID)
			row, ok := byID[loanOID]
			// Khoản vay chưa kịp project: bỏ qua dòng này thay vì làm hỏng cả trang.
			if !ok || !validProjectedContract(row, app.Application) {
				continue
			}
			count, oldest := overdueOf(row, app.ID)
			if f.Overdue && count == 0 || f.Status != "" && f.Status != row.Status {
				continue
			}
			if len(page.Items) == limit {
				// Còn dòng hợp lệ nên có trang sau, bắt đầu ngay sau dòng cuối đã trả.
				encoded, err := encodeAdminLoanCursor(lastReturned)
				if err != nil {
					return AdminLoanContractPage{}, err
				}
				page.NextCursor = &encoded
				return page, nil
			}
			page.Items = append(page.Items, AdminLoanContract{
				ContractSummary: contractSummary(app.Application, row), CustomerID: app.OwnerID,
				OverdueInstallments: count, OldestOverdueDate: oldest,
			})
			lastReturned = after
		}
		if len(apps) < adminLoanScanBatch {
			return page, nil
		}
	}
}

func encodeAdminLoanCursor(c adminLoanCursor) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// Detail tra chủ hồ sơ rồi dùng lại đúng các truy vấn của khách.
func (q *AdminLoanContracts) Detail(ctx context.Context, loanID string) (AdminLoanContractDetail, error) {
	var record struct {
		OwnerID string `bson:"owner_id"`
	}
	err := q.db.Collection("loan_applications_read").FindOne(ctx, bson.M{
		"_id": loanID, "status": string(loandomain.ApplicationStatusDisbursed),
	}).Decode(&record)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return AdminLoanContractDetail{}, loanqueries.ErrLoanContractNotFound
	}
	if err != nil {
		return AdminLoanContractDetail{}, err
	}
	detail, err := q.contracts.ReadContract(ctx, loanID, record.OwnerID)
	if err != nil {
		return AdminLoanContractDetail{}, err
	}
	payments, err := q.payments.ReadPayments(ctx, loanID, record.OwnerID)
	if err != nil {
		return AdminLoanContractDetail{}, err
	}
	if payments == nil {
		payments = []loanqueries.Payment{}
	}
	return AdminLoanContractDetail{ContractDetail: detail, CustomerID: record.OwnerID, Payments: payments}, nil
}
