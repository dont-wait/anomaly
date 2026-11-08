package mongo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// profileChangeReviewRecord is authoritative. The customer.profile_change
// field is only a compatibility mirror for the existing customer UI.
type profileChangeReviewRecord struct {
	ID              string                      `bson:"_id"`
	AccountID       string                      `bson:"account_id"`
	CustomerID      bson.ObjectID               `bson:"customer_id"`
	Username        string                      `bson:"username"`
	SearchText      string                      `bson:"search_text"`
	Status          string                      `bson:"status"`
	RequestedAt     time.Time                   `bson:"requested_at"`
	Proposed        profileChangeRecord         `bson:"proposed"`
	Previous        profileChangeSnapshotRecord `bson:"previous"`
	HasPrevious     bool                        `bson:"has_previous"`
	ReviewedAt      *time.Time                  `bson:"reviewed_at,omitempty"`
	ReviewedBy      string                      `bson:"reviewed_by,omitempty"`
	RejectionReason string                      `bson:"rejection_reason,omitempty"`
}

type profileChangeSnapshotRecord struct {
	FullName         string `bson:"full_name"`
	AddressLine      string `bson:"address_line"`
	ProvinceCode     string `bson:"province_code"`
	IssuedPlace      string `bson:"issued_place"`
	PermanentAddress string `bson:"permanent_address"`
}

func snapshotOfCustomer(customer customerRecord) profileChangeSnapshotRecord {
	return profileChangeSnapshotRecord{
		FullName:         customer.Profile.FullName,
		AddressLine:      customer.Profile.Address.Line,
		ProvinceCode:     customer.Profile.Address.ProvinceCode,
		IssuedPlace:      stringValue(customer.Identity.IssuedPlace),
		PermanentAddress: stringValue(customer.Identity.PermanentAddress),
	}
}

func (s profileChangeSnapshotRecord) domain() accountdomain.ProfileChangeSnapshot {
	return accountdomain.ProfileChangeSnapshot{
		FullName: s.FullName, AddressLine: s.AddressLine, ProvinceCode: s.ProvinceCode,
		IssuedPlace: s.IssuedPlace, PermanentAddress: s.PermanentAddress,
	}
}

func (r profileChangeReviewRecord) domain() accountdomain.ProfileChangeReview {
	change := fromProfileChangeRecord(&r.Proposed)
	// Status and review details are stored at the top level for indexed queries.
	change.Status = r.Status
	change.RejectionReason = r.RejectionReason
	change.ReviewedAt = r.ReviewedAt
	change.ReviewedBy = r.ReviewedBy
	return accountdomain.ProfileChangeReview{
		AccountID: r.AccountID, Username: r.Username,
		Previous: r.Previous.domain(), HasPrevious: r.HasPrevious, Change: *change,
	}
}

// EnsureProfileChangeIndexes is safe to call after migration 23. It does not
// create/backfill the collection: deployment must run migrations first.
func (r *AccountAggregateRepository) EnsureProfileChangeIndexes(ctx context.Context) error {
	// Fail closed when the migration has not created/backfilled the review store.
	names, err := r.profileChangeReviews.Database().ListCollectionNames(ctx, bson.M{"name": "profile_change_requests"})
	if err != nil {
		return err
	}
	if len(names) != 1 {
		return errors.New("profile_change_requests collection missing; run migration 23 before starting API")
	}
	indexes := []mongodrv.IndexModel{
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "requested_at", Value: -1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName("idx_review_status_newest")},
		{Keys: bson.D{{Key: "requested_at", Value: -1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName("idx_review_newest")},
		{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "requested_at", Value: -1}}, Options: options.Index().SetName("idx_review_account")},
		{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "status", Value: 1}}, Options: options.Index().SetName("uniq_review_pending_account").SetUnique(true).SetPartialFilterExpression(bson.M{"status": accountdomain.ProfileChangePending})},
	}
	_, err = r.profileChangeReviews.Indexes().CreateMany(ctx, indexes)
	return err
}

// SubmitProfileChange stores the request, the before-image and the customer UI
// mirror atomically. A new submission supersedes any older pending request.
func (r *AccountAggregateRepository) SubmitProfileChange(ctx context.Context, account *accountdomain.UserAccount, request *accountdomain.ProfileChangeRequest) error {
	customerID, err := bson.ObjectIDFromHex(account.CustomerId)
	if err != nil {
		return err
	}
	accountID, err := accountRecordID(account.Id)
	if err != nil {
		return err
	}
	session, err := r.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		var customer customerRecord
		if err := r.customers.col.FindOne(tx, bson.M{"_id": customerID}).Decode(&customer); err != nil {
			if errors.Is(err, mongodrv.ErrNoDocuments) {
				return nil, accountdomain.ErrAccountNotFound
			}
			return nil, err
		}
		// Ensure a transaction write conflict on concurrent submits/decisions.
		changed, err := r.customers.col.UpdateOne(tx, bson.M{"_id": customerID}, bson.M{"$set": bson.M{
			"profile_change": toProfileChangeRecord(request), "updated_at": request.RequestedAt,
		}})
		if err != nil {
			return nil, err
		}
		if changed.MatchedCount != 1 {
			return nil, accountdomain.ErrAccountNotFound
		}
		_, err = r.profileChangeReviews.UpdateMany(tx, bson.M{"account_id": account.Id, "status": accountdomain.ProfileChangePending}, bson.M{"$set": bson.M{"status": accountdomain.ProfileChangeSuperseded, "proposed.status": accountdomain.ProfileChangeSuperseded}})
		if err != nil {
			return nil, err
		}
		review := profileChangeReviewRecord{
			ID: request.Id, AccountID: account.Id, CustomerID: customerID, Username: account.Username,
			SearchText: strings.ToLower(strings.Join([]string{account.Id, account.Username, customer.Profile.FullName, request.FullName}, " ")),
			Status:     accountdomain.ProfileChangePending, RequestedAt: request.RequestedAt,
			Proposed: *toProfileChangeRecord(request), Previous: snapshotOfCustomer(customer), HasPrevious: true,
		}
		if _, err = r.profileChangeReviews.InsertOne(tx, review); err != nil {
			return nil, err
		}
		_, err = r.accounts.col.UpdateOne(tx, bson.M{"_id": accountID}, bson.M{"$inc": bson.M{"version": 1}, "$set": bson.M{"updated_at": request.RequestedAt}})
		return nil, err
	})
	return err
}

// DecideProfileChange uses a pending-only compare-and-swap. Both the review
// decision and selective profile edits commit in the SAME Mongo transaction.
func (r *AccountAggregateRepository) DecideProfileChange(ctx context.Context, adminID, accountID, requestID, decision, reason string, reviewedAt time.Time) error {
	session, err := r.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		filter := bson.M{"_id": requestID, "account_id": accountID, "status": accountdomain.ProfileChangePending}
		// First take the pending request with an atomic conditional update.
		update := bson.M{"$set": bson.M{"status": decision, "proposed.status": decision,
			"reviewed_at": reviewedAt, "reviewed_by": adminID, "rejection_reason": reason,
			"proposed.reviewed_at": reviewedAt, "proposed.reviewed_by": adminID, "proposed.rejection_reason": reason}}
		var review profileChangeReviewRecord
		err := r.profileChangeReviews.FindOneAndUpdate(tx, filter, update).Decode(&review)
		if errors.Is(err, mongodrv.ErrNoDocuments) {
			return nil, accountdomain.ErrProfileChangeClosed
		}
		if err != nil {
			return nil, err
		}
		customerUpdate := bson.M{
			// Rebuild the UI mirror from the authoritative review record.
			"profile_change": toProfileChangeRecord(&accountdomain.ProfileChangeRequest{
				Id: review.ID, FullName: review.Proposed.FullName, AddressLine: review.Proposed.AddressLine,
				ProvinceCode: review.Proposed.ProvinceCode, IssuedPlace: review.Proposed.IssuedPlace,
				PermanentAddress: review.Proposed.PermanentAddress, Status: decision,
				RequestedAt: review.RequestedAt, ReviewedAt: &reviewedAt, ReviewedBy: adminID,
				RejectionReason: reason,
			}),
			"updated_at": reviewedAt,
		}

		if decision == accountdomain.ProfileChangeApproved {
			// Only changed fields are written; unrelated customer attributes
			// (including KYC/media) are never replaced by a stale aggregate.
			for _, field := range []struct{ value, key string }{
				{review.Proposed.FullName, "profile.full_name"},
				{review.Proposed.AddressLine, "profile.address.line"},
				{review.Proposed.ProvinceCode, "profile.address.province_code"},
				{review.Proposed.IssuedPlace, "identity.issued_place"},
				{review.Proposed.PermanentAddress, "identity.permanent_address"},
			} {
				if field.value != "" {
					customerUpdate[field.key] = field.value
				}
			}
		}
		result, err := r.customers.col.UpdateOne(tx, bson.M{"_id": review.CustomerID}, bson.M{"$set": customerUpdate})
		if err != nil {
			return nil, err
		}
		if result.MatchedCount != 1 {
			return nil, accountdomain.ErrProfileChangeClosed
		}
		accountObjectID, err := accountRecordID(accountID)
		if err != nil {
			return nil, err
		}
		result, err = r.accounts.col.UpdateOne(tx, bson.M{"_id": accountObjectID}, bson.M{"$inc": bson.M{"version": 1}, "$set": bson.M{"updated_at": reviewedAt}})
		if err != nil {
			return nil, err
		}
		if result.MatchedCount != 1 {
			return nil, accountdomain.ErrAccountNotFound
		}
		return nil, nil
	})
	return err
}

type reviewCursor struct {
	At     time.Time `json:"at"`
	ID     string    `json:"id"`
	Status string    `json:"status"`
	Search string    `json:"search"`
}

func encodeReviewCursor(at time.Time, id, status, search string) string {
	raw, _ := json.Marshal(reviewCursor{At: at, ID: id, Status: status, Search: search})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func decodeReviewCursor(raw, status, search string) (reviewCursor, error) {
	if raw == "" {
		return reviewCursor{}, nil
	}
	if len(raw) > 1024 {
		return reviewCursor{}, accountdomain.ErrInvalidProfile
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return reviewCursor{}, accountdomain.ErrInvalidProfile
	}
	var cursor reviewCursor
	if err = json.Unmarshal(data, &cursor); err != nil || cursor.At.IsZero() || cursor.ID == "" || cursor.Status != status || cursor.Search != search {
		return reviewCursor{}, accountdomain.ErrInvalidProfile
	}
	return cursor, nil
}

// ListProfileChanges queries indexed request records without hydrating all
// accounts. The cursor makes later pages stable as NEW requests are inserted.
func (r *AccountAggregateRepository) ListProfileChanges(ctx context.Context, options accountdomain.ProfileChangeListOptions) (accountdomain.ProfileChangeListResult, error) {
	result := accountdomain.ProfileChangeListResult{Items: make([]accountdomain.ProfileChangeReview, 0)}
	if options.PageSize < 1 || options.PageSize > 50 || options.Page < 1 {
		return result, accountdomain.ErrInvalidProfile
	}
	search := strings.ToLower(strings.TrimSpace(options.Search))
	cursor, err := decodeReviewCursor(options.Cursor, options.Status, search)
	if err != nil {
		return result, err
	}
	clauses := make([]bson.M, 0, 3)
	if options.Status != "ALL" {
		clauses = append(clauses, bson.M{"status": options.Status})
	}
	if search != "" {
		clauses = append(clauses, bson.M{"search_text": bson.M{"$regex": regexp.QuoteMeta(search)}})
	}
	base := bson.M{}
	if len(clauses) > 0 {
		base["$and"] = clauses
	}
	count, err := r.profileChangeReviews.CountDocuments(ctx, base)
	if err != nil {
		return result, err
	}
	if count > math.MaxInt {
		return result, fmt.Errorf("too many profile change reviews")
	}
	result.Total = int(count)
	if cursor.ID != "" {
		clauses = append(clauses, bson.M{"$or": bson.A{
			bson.M{"requested_at": bson.M{"$lt": cursor.At}},
			bson.M{"requested_at": cursor.At, "_id": bson.M{"$gt": cursor.ID}},
		}})
	}
	filter := bson.M{}
	if len(clauses) > 0 {
		filter["$and"] = clauses
	}
	opts := profileReviewFindOptions(options.PageSize + 1)
	if cursor.ID == "" && options.Page > 1 {
		// Older callers can still use offset pages; new UI always uses cursor.
		opts.SetSkip(int64(options.Page-1) * int64(options.PageSize))
	}
	docs, err := r.profileChangeReviews.Find(ctx, filter, opts)
	if err != nil {
		return result, err
	}
	defer docs.Close(ctx)
	records := make([]profileChangeReviewRecord, 0, options.PageSize+1)
	if err := docs.All(ctx, &records); err != nil {
		return result, err
	}
	hasNext := len(records) > options.PageSize
	if hasNext {
		records = records[:options.PageSize]
	}
	for _, record := range records {
		result.Items = append(result.Items, record.domain())
	}
	if hasNext && len(records) > 0 {
		last := records[len(records)-1]
		result.NextCursor = encodeReviewCursor(last.RequestedAt, last.ID, options.Status, search)
	}
	return result, nil
}

func profileReviewFindOptions(limit int) *options.FindOptionsBuilder {
	return options.Find().SetSort(bson.D{{Key: "requested_at", Value: -1}, {Key: "_id", Value: 1}}).SetLimit(int64(limit))
}

func (r *AccountAggregateRepository) currentProfileChange(ctx context.Context, accountID string) (*accountdomain.ProfileChangeRequest, error) {
	var rec profileChangeReviewRecord
	err := r.profileChangeReviews.FindOne(ctx, bson.M{"account_id": accountID}, options.FindOne().SetSort(bson.D{{Key: "requested_at", Value: -1}, {Key: "_id", Value: 1}})).Decode(&rec)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	change := rec.domain().Change
	return &change, nil
}
