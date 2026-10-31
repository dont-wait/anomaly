package mongo

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSavingsFeedTypeAcceptedByExistingMigrations(t *testing.T) {
	for _, filename := range []string{
		"000005_create_transaction_collections.up.json",
		"000009_allow_loan_disbursement_feed.up.json",
		"000010_allow_loan_repayment.up.json",
	} {
		t.Run(filename, func(t *testing.T) {
			data, err := os.ReadFile("../../../migrations/" + filename)
			if err != nil {
				t.Fatal(err)
			}
			var steps []map[string]any
			if err := json.Unmarshal(data, &steps); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, step := range steps {
				if step["collMod"] != "account_transaction_feed" {
					continue
				}
				found = true
				validator := step["validator"].(map[string]any)
				schema := validator["$jsonSchema"].(map[string]any)
				properties := schema["properties"].(map[string]any)
				types := properties["type"].(map[string]any)["enum"].([]any)
				allowed := false
				for _, kind := range types {
					if kind == savingsFeedType {
						allowed = true
					}
				}
				if !allowed {
					t.Fatalf("%s rejects feed type %q", filename, savingsFeedType)
				}
			}
			if !found {
				t.Fatalf("%s does not declare a transaction feed validator", filename)
			}
		})
	}
}
