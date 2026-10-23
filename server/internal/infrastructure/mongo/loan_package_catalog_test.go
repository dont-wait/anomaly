package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPackageMoneyToInt64(t *testing.T) {
	samples := []struct {
		input any
		want  int64
		valid bool
	}{
		{int64(12000000), 12000000, true},
		{int32(12000000), 12000000, true},
		{"bad", 0, false},
	}
	decimal, err := bson.ParseDecimal128("500000000.00")
	if err != nil {
		t.Fatal(err)
	}
	samples = append(samples, struct {
		input any
		want  int64
		valid bool
	}{decimal, 500000000, true})
	fraction, err := bson.ParseDecimal128("1.25")
	if err != nil {
		t.Fatal(err)
	}
	samples = append(samples, struct {
		input any
		want  int64
		valid bool
	}{fraction, 0, false})
	for _, tt := range samples {
		encoded, err := bson.Marshal(bson.M{"amount": tt.input})
		if err != nil {
			t.Fatal(err)
		}
		got, err := packageMoneyToInt64(bson.Raw(encoded).Lookup("amount"))
		if (err == nil) != tt.valid || (err == nil && got != tt.want) {
			t.Fatalf("input=%v got=%d err=%v want=%d valid=%v", tt.input, got, err, tt.want, tt.valid)
		}
	}
}
