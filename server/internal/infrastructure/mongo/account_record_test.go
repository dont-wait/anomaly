package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestAccountRecordSupportsCurrentAndLegacyIDs(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		wantString bool
	}{
		{name: "current ObjectID", id: "507f1f77bcf86cd799439011"},
		{name: "legacy hex ID", id: "0123456789abcdef0123456789abcdef", wantString: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &accountdomain.UserAccount{
				Id:         tt.id,
				CustomerId: "507f191e810c19729de860ea",
			}
			record, err := toRecord(account)
			if err != nil {
				t.Fatalf("toRecord() error = %v", err)
			}
			_, isString := record.Id.(string)
			if isString != tt.wantString {
				t.Errorf("record ID type = %T, wantString = %v", record.Id, tt.wantString)
			}
			if !tt.wantString {
				if _, ok := record.Id.(bson.ObjectID); !ok {
					t.Errorf("record ID type = %T, want bson.ObjectID", record.Id)
				}
			}

			got, err := fromRecord(record)
			if err != nil {
				t.Fatalf("fromRecord() error = %v", err)
			}
			if got.Id != tt.id {
				t.Errorf("round-trip account ID = %q, want %q", got.Id, tt.id)
			}
		})
	}
}
