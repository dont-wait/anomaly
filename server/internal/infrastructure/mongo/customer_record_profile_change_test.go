package mongo

import (
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestCustomerRecordPersistsProfileChange(t *testing.T) {
	requested := time.Date(2026, 11, 7, 12, 34, 56, 0, time.UTC)
	reviewed := requested.Add(time.Hour)
	for _, scenario := range []struct {
		name   string
		status string
	}{
		{"pending", accountdomain.ProfileChangePending},
		{"approved", accountdomain.ProfileChangeApproved},
		{"rejected", accountdomain.ProfileChangeRejected},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			change := &accountdomain.ProfileChangeRequest{
				Id: "request-123", FullName: "Tên mới", AddressLine: "Đường A",
				ProvinceCode: "HN", IssuedPlace: "Hà Nội", PermanentAddress: "Đường B",
				Status: scenario.status, RequestedAt: requested,
			}
			if scenario.status != accountdomain.ProfileChangePending {
				change.ReviewedBy = "admin-1"
				change.ReviewedAt = &reviewed
			}
			if scenario.status == accountdomain.ProfileChangeRejected {
				change.RejectionReason = "Không hợp lệ"
			}
			customer := &accountdomain.Customer{
				Id:            bson.NewObjectID().Hex(),
				Profile:       accountdomain.CustomerProfile{FullName: "Tên cũ"},
				ProfileChange: change,
			}
			record, err := toCustomerRecord(customer)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := bson.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			var decoded customerRecord
			if err := bson.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			result := fromCustomerRecord(decoded)
			if result.Profile.FullName != "Tên cũ" {
				t.Fatalf("profile changed before approval: %q", result.Profile.FullName)
			}
			got := result.ProfileChange
			if got == nil || got.Id != change.Id || got.FullName != change.FullName ||
				got.AddressLine != change.AddressLine || got.ProvinceCode != change.ProvinceCode ||
				got.IssuedPlace != change.IssuedPlace || got.PermanentAddress != change.PermanentAddress ||
				got.Status != change.Status || got.RejectionReason != change.RejectionReason ||
				got.ReviewedBy != change.ReviewedBy || !got.RequestedAt.Equal(change.RequestedAt) {
				t.Fatalf("profile change not persisted: %#v", got)
			}
			if (got.ReviewedAt == nil) != (change.ReviewedAt == nil) {
				t.Fatalf("review time lost: %#v", got.ReviewedAt)
			}
			if got.ReviewedAt != nil && !got.ReviewedAt.Equal(*change.ReviewedAt) {
				t.Fatalf("review time mismatch: %v", got.ReviewedAt)
			}
		})
	}
}

func TestCustomerRecordWithoutProfileChange(t *testing.T) {
	customer := &accountdomain.Customer{Id: bson.NewObjectID().Hex()}
	record, err := toCustomerRecord(customer)
	if err != nil {
		t.Fatal(err)
	}
	data, err := bson.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var decoded customerRecord
	if err := bson.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := fromCustomerRecord(decoded).ProfileChange; got != nil {
		t.Fatalf("expected missing legacy field to stay nil, got %#v", got)
	}
}
