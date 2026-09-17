package main

import (
	"testing"

	"github.com/dont-wait/anomaly/internal/seeder"
)

func TestSeedRequiresLocalOptIn(t *testing.T) {
	for _, tc := range []struct {
		env, enabled string
		allowed      bool
	}{
		{"production", "true", false},
		{"", "true", false},
		{"local", "false", false},
		{"development", "", false},
		{"local", "true", true},
		{"development", "true", true},
		{"dev", "true", true},
	} {
		t.Run(tc.env+"/"+tc.enabled, func(t *testing.T) {
			err := seeder.ValidateEnvironment(tc.env, tc.enabled)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, error=%v", tc.allowed, err)
			}
		})
	}
}
