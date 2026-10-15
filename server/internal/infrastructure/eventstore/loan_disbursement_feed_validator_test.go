package eventstore_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestLoanDisbursementFeedValidatorMigration(t *testing.T) {
	read := func(filename string) map[string]any {
		t.Helper()

		data, err := os.ReadFile(
			"../../../migrations/" + filename,
		)
		if err != nil {
			t.Fatal(err)
		}

		var commands []map[string]any
		if err := json.Unmarshal(data, &commands); err != nil {
			t.Fatal(err)
		}

		for _, command := range commands {
			if command["collMod"] == "account_transaction_feed" {
				return command
			}
		}

		t.Fatalf("feed validator not found in %s", filename)
		return nil
	}

	original := read(
		"000005_create_transaction_collections.up.json",
	)

	up := read(
		"000007_allow_loan_disbursement_feed.up.json",
	)

	down := read(
		"000007_allow_loan_disbursement_feed.down.json",
	)

	// DOWN phải khôi phục đúng validator cũ.
	if !reflect.DeepEqual(original, down) {
		t.Fatal("DOWN does not restore original feed validator")
	}

	// Chỉ bổ sung loan_disbursement vào danh sách type.
	schema := original["validator"].(map[string]any)["$jsonSchema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	typeRule := properties["type"].(map[string]any)

	allowed := typeRule["enum"].([]any)
	typeRule["enum"] = append(
		append([]any{}, allowed...),
		"loan_disbursement",
	)

	// UP không được làm mất bất kỳ quy tắc validation nào.
	if !reflect.DeepEqual(original, up) {
		t.Fatal("UP changes more than the allowed feed types")
	}
}
