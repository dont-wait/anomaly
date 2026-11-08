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
	SearchKeys      []string                    `bson:"search_keys,omitempty"`
	Email           string                      `bson:"email,omitempty"`
	IdentityType    string                      `bson:"identity_type,omitempty"`
	IdentityMasked  string                      `bson:"identity_number_masked,omitempty"`
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
		AccountID: r.AccountID, Username: r.Username, Email: r.Email,
		IdentityType: r.IdentityType, IdentityNumberMasked: r.IdentityMasked,
		Previous: r.Previous.domain(), HasPrevious: r.HasPrevious, Change: *change,
	}
}

// VerifyProfileChangeMigration only checks the collection created by migration
// 24. Index creation and backfill are owned exclusively by migrations.
// The API does not fail startup if deployment order is wrong; review methods
// return an explicit error until migrations are applied.
func (r *AccountAggregateRepository) VerifyProfileChangeMigration(ctx context.Context) error {
	if r.reviewReady.Load() {
		return nil
	}
	names, err := r.profileChangeReviews.Database().ListCollectionNames(ctx, bson.M{"name": "profile_change_requests"})
	if err != nil {
		return err
	}
	if len(names) != 1 {
		return fmt.Errorf("%w: profile_change_requests missing; run migration 23", accountdomain.ErrProfileReviewUnavailable)
	}
	indexes, err := r.profileChangeReviews.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer indexes.Close(ctx)
	var entries []struct {
		Name string `bson:"name"`
	}
	if err = indexes.All(ctx, &entries); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name == "idx_review_search_keys" {
			r.reviewReady.Store(true)
			return nil
		}
	}
	return fmt.Errorf("%w: search index missing; run migration 24", accountdomain.ErrProfileReviewUnavailable)
}

// Prefix search is index-backed. Full names and individual words are indexed,
// so "Văn" and "Nguyễn Văn" both find "Nguyễn Văn A" without collection scans.
func reviewSearchKeys(values ...string) []string {
	keys := make([]string, 0, 16)
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		keys = append(keys, value)
	}
	for _, value := range values {
		words := strings.Fields(value)
		for i := range words {
			add(strings.Join(words[i:], " "))
		}
	}
	return keys
}

func maskedIdentity(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-4:])
}

// SubmitProfileChange stores the request, the before-image and the customer UI
// mirror atomically. A new submission supersedes any older pending request.
func (r *AccountAggregateRepository) SubmitProfileChange(ctx context.Context, account *accountdomain.UserAccount, request *accountdomain.ProfileChangeRequest) error {
	if err := r.VerifyProfileChangeMigration(ctx); err != nil {
		return err
	}
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
			Email: customer.Profile.Email, IdentityType: stringValue(customer.Identity.Type),
			IdentityMasked: maskedIdentity(stringValue(customer.Identity.Number)),
			SearchText:     strings.ToLower(strings.Join([]string{account.Id, account.Username, customer.Profile.FullName, request.FullName}, " ")),
			SearchKeys:     reviewSearchKeys(account.Id, account.Username, customer.Profile.FullName, request.FullName),
			Status:         accountdomain.ProfileChangePending, RequestedAt: request.RequestedAt,
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
	if err := r.VerifyProfileChangeMigration(ctx); err != nil {
		return err
	}
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
			// Keep search metadata consistent with the new approved name.
			approvedName := review.Previous.FullName
			if review.Proposed.FullName != "" {
				approvedName = review.Proposed.FullName
			}
			searchKeys := reviewSearchKeys(review.AccountID, review.Username, review.Previous.FullName, approvedName, review.Proposed.FullName)
			searchText := strings.ToLower(strings.Join([]string{review.AccountID, review.Username, review.Previous.FullName, approvedName, review.Proposed.FullName}, " "))
			if _, err := r.profileChangeReviews.UpdateOne(tx,
				bson.M{"_id": review.ID, "status": decision},
				bson.M{"$set": bson.M{"search_keys": searchKeys, "search_text": searchText}}); err != nil {
				return nil, err
			}
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

// ListProfileChanges uses indexed prefix terms and one aggregation. A $facet
// computes an exact total and the bounded page from the SAME read operation.
// Requests added after the cursor boundary cannot shift later pages.
func (r *AccountAggregateRepository) ListProfileChanges(ctx context.Context, filterOptions accountdomain.ProfileChangeListOptions) (accountdomain.ProfileChangeListResult, error) {
	result := accountdomain.ProfileChangeListResult{Items: make([]accountdomain.ProfileChangeReview, 0)}
	if err := r.VerifyProfileChangeMigration(ctx); err != nil {
		return result, err
	}
	if filterOptions.PageSize < 1 || filterOptions.PageSize > 50 || filterOptions.Page < 1 {
		return result, accountdomain.ErrInvalidProfile
	}
	search := strings.ToLower(strings.TrimSpace(filterOptions.Search))
	cursor, err := decodeReviewCursor(filterOptions.Cursor, filterOptions.Status, search)
	if err != nil {
		return result, err
	}

	match := bson.M{}
	if filterOptions.Status != "ALL" {
		match["status"] = filterOptions.Status
	}
	if search != "" {
		match["search_keys"] = bson.M{"$regex": "^" + regexp.QuoteMeta(search)}
	}

	items := bson.A{}
	if cursor.ID != "" {
		items = append(items, bson.D{{Key: "$match", Value: bson.M{"$or": bson.A{
			bson.M{"requested_at": bson.M{"$lt": cursor.At}},
			bson.M{"requested_at": cursor.At, "_id": bson.M{"$gt": cursor.ID}},
		}}}})
	} else if filterOptions.Page > 1 {
		// Compatibility for older clients calling the page API with offsets.
		items = append(items, bson.D{{Key: "$skip", Value: int64(filterOptions.Page-1) * int64(filterOptions.PageSize)}})
	}
	items = append(items, bson.D{{Key: "$limit", Value: int64(filterOptions.PageSize + 1)}})
	pipeline := mongodrv.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "requested_at", Value: -1}, {Key: "_id", Value: 1}}}},
		bson.D{{Key: "$facet", Value: bson.M{
			"meta":  bson.A{bson.D{{Key: "$count", Value: "total"}}},
			"items": items,
		}}},
	}
	stream, err := r.profileChangeReviews.Aggregate(ctx, pipeline)
	if err != nil {
		return result, err
	}
	defer stream.Close(ctx)
	var output []struct {
		Meta []struct {
			Total int64 `bson:"total"`
		} `bson:"meta"`
		Items []profileChangeReviewRecord `bson:"items"`
	}
	if err := stream.All(ctx, &output); err != nil {
		return result, err
	}
	if len(output) != 1 {
		return result, errors.New("invalid profile change aggregation result")
	}
	if len(output[0].Meta) != 0 {
		if output[0].Meta[0].Total > math.MaxInt {
			return result, fmt.Errorf("too many profile change reviews")
		}
		result.Total = int(output[0].Meta[0].Total)
	}
	records := output[0].Items
	hasNext := len(records) > filterOptions.PageSize
	if hasNext {
		records = records[:filterOptions.PageSize]
	}
	for _, record := range records {
		result.Items = append(result.Items, record.domain())
	}
	if hasNext && len(records) != 0 {
		last := records[len(records)-1]
		result.NextCursor = encodeReviewCursor(last.RequestedAt, last.ID, filterOptions.Status, search)
	}
	return result, nil
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
