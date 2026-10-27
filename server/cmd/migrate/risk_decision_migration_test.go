package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRiskDecisionMigrationCreatesCollectionsAndIndexes(t *testing.T) {
	path := filepath.Join("../../migrations", "000011_create_risk_decisions.up.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var commands []struct {
		Create        string `json:"create"`
		CreateIndexes string `json:"createIndexes"`
		Indexes       []struct {
			Name            string `json:"name"`
			ExpireAfterSecs *int   `json:"expireAfterSeconds"`
		} `json:"indexes"`
	}
	if err := json.Unmarshal(body, &commands); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 {
		t.Fatalf("migration has %d commands, want 4", len(commands))
	}
	if commands[0].Create != "risk_decisions" || commands[2].Create != "risk_decision_rejections" {
		t.Fatalf("collections = %q and %q", commands[0].Create, commands[2].Create)
	}
	if commands[1].CreateIndexes != "risk_decisions" || commands[3].CreateIndexes != "risk_decision_rejections" {
		t.Fatalf("index targets = %q and %q", commands[1].CreateIndexes, commands[3].CreateIndexes)
	}
	if got := len(commands[1].Indexes); got != 2 {
		t.Fatalf("risk_decisions indexes = %d, want 2", got)
	}
	if commands[1].Indexes[0].Name != "uniq_risk_decision_transfer" || commands[1].Indexes[1].Name != "ttl_risk_decision" {
		t.Fatalf("risk_decisions index names = %+v", commands[1].Indexes)
	}
	if got := len(commands[3].Indexes); got != 1 || commands[3].Indexes[0].Name != "ttl_risk_decision_rejections" {
		t.Fatalf("rejection indexes = %+v", commands[3].Indexes)
	}
}
