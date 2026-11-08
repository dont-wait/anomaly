package mongo

import (
	"errors"
	"strings"
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestProfileReviewCursorRejectsWrongFilter(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	cursor := encodeReviewCursor(now, "request-a", "PENDING", "nguyen")
	got, err := decodeReviewCursor(cursor, "PENDING", "nguyen")
	if err != nil || got.ID != "request-a" || !got.At.Equal(now) {
		t.Fatalf("cursor %#v: %v", got, err)
	}
	for _, test := range []struct{ raw, status, search string }{
		{cursor, "APPROVED", "nguyen"}, {cursor, "PENDING", "other"},
		{"not base64", "PENDING", "nguyen"}, {strings.Repeat("a", 1025), "PENDING", "nguyen"},
	} {
		if _, err := decodeReviewCursor(test.raw, test.status, test.search); !errors.Is(err, accountdomain.ErrInvalidProfile) {
			t.Errorf("accepted %q: %v", test.raw, err)
		}
	}
}

func TestProfileReviewStoresBeforeImageAndStatus(t *testing.T) {
	old := profileChangeSnapshotRecord{FullName: "Tên cũ", AddressLine: "Đường cũ", ProvinceCode: "HCM"}
	proposed := profileChangeRecord{ID: "request-1", FullName: "Tên mới", Status: "PENDING"}
	rec := profileChangeReviewRecord{ID: "request-1", AccountID: "a1", Status: "APPROVED", Proposed: proposed, Previous: old, HasPrevious: true}
	data, err := bson.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var decoded profileChangeReviewRecord
	if err := bson.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	got := decoded.domain()
	if got.Previous.FullName != "Tên cũ" || got.Change.FullName != "Tên mới" || got.Change.Status != "APPROVED" || !got.HasPrevious {
		t.Fatalf("review %#v", got)
	}
}

func TestRoutineCustomerSavePreservesProfileChangeMirror(t *testing.T) {
	update := customerProfileUpdate(customerRecord{CustomerCode: "C001"})
	set := update["$set"].(bson.M)
	if _, ok := set["profile_change"]; ok {
		t.Fatal("routine save must not overwrite a pending profile change")
	}
	if _, ok := update["$replaceRoot"]; ok {
		t.Fatal("routine save unexpectedly replaces document")
	}
}

func TestAccountRecordIDAcceptsObjectIDAndLegacyHexID(t *testing.T) {
	for _, id := range []string{bson.NewObjectID().Hex(), "abcdefabcdefabcdefabcdefabcdefab"} {
		recordID, err := accountRecordID(id)
		if err != nil {
			t.Fatalf("account %q: %v", id, err)
		}
		switch value := recordID.(type) {
		case bson.ObjectID:
			if value.Hex() != id {
				t.Fatalf("object id %v", value)
			}
		case string:
			if value != id {
				t.Fatalf("string id %q", value)
			}
		default:
			t.Fatalf("unsupported ID type %T", recordID)
		}
	}
}

func TestReviewSearchKeysAndIdentityMask(t *testing.T) {
	keys := reviewSearchKeys("ACC123", "khach", "Nguyễn Văn A", "Trần Văn B")
	for _, expected := range []string{"acc123", "nguyễn văn a", "văn a", "a", "trần văn b", "văn b"} {
		found := false
		for _, key := range keys {
			if key == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing indexed prefix %q in %q", expected, keys)
		}
	}
	if got := maskedIdentity("123456789012"); got != "********9012" {
		t.Fatalf("identity mask leaked or lost suffix: %q", got)
	}
	if got := maskedIdentity("1234"); got != "****" {
		t.Fatalf("short identity: %q", got)
	}
}
