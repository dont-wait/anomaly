package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestToKYCSessionRecordIncludesLivenessVideoMediaFields(t *testing.T) {
	session := &accountdomain.KYCSession{
		Id:         "507f1f77bcf86cd799439011",
		CustomerId: "507f191e810c19729de860ea",
		Media: accountdomain.KYCMedia{
			LivenessVideo: accountdomain.LivenessVideo{
				MediaObject: accountdomain.MediaObject{
					StorageKey: "kyc/liveness.mp4",
					MIMEType:   "video/mp4",
					SizeBytes:  1234,
					SHA256:     "video-sha256",
				},
				DurationSeconds: 8,
			},
		},
	}

	record, err := toKYCSessionRecord(session)
	if err != nil {
		t.Fatalf("toKYCSessionRecord() error = %v", err)
	}

	data, err := bson.Marshal(record.Media.LivenessVideo)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}
	raw := bson.Raw(data)

	assertString := func(key, want string) {
		t.Helper()
		got, ok := raw.Lookup(key).StringValueOK()
		if !ok || got != want {
			t.Errorf("BSON field %q = %q, %v; want %q, true", key, got, ok, want)
		}
	}
	assertString("storage_key", "kyc/liveness.mp4")
	assertString("mime_type", "video/mp4")
	assertString("sha256", "video-sha256")

	if got, ok := raw.Lookup("size_bytes").Int64OK(); !ok || got != 1234 {
		t.Errorf("BSON field size_bytes = %d, %v; want 1234, true", got, ok)
	}
}
