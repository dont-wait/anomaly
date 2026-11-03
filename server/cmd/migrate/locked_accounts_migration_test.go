package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestLockedAccountsMigrationOnlyChangesStatusEnum(t *testing.T) {
	read := func(path string) []map[string]any {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var commands []map[string]any
		if err := json.Unmarshal(data, &commands); err != nil {
			t.Fatal(err)
		}
		return commands
	}
	baseline := read("../../migrations/000004_add_account_role.up.json")[1]
	up := read("../../migrations/000017_allow_locked_accounts.up.json")
	down := read("../../migrations/000017_allow_locked_accounts.down.json")
	if len(up) != 1 || len(down) != 1 {
		t.Fatal("migration must only modify the accounts validator")
	}
	if !reflect.DeepEqual(down[0], baseline) {
		t.Fatal("down migration must restore the previous validator without updating account data")
	}
	properties := baseline["validator"].(map[string]any)["$jsonSchema"].(map[string]any)["properties"].(map[string]any)
	properties["status"] = map[string]any{"enum": []any{"active", "locked"}}
	if !reflect.DeepEqual(up[0], baseline) {
		t.Fatal("up migration must preserve existing validation except the status enum")
	}
}
