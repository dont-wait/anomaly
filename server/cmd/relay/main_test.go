package main

import (
	"testing"
)

func TestSanitizePayloadRemovesSensitiveFields(t *testing.T) {
	got, err := sanitizePayload([]byte(`{"account":{"PasswordHash":"secret","username":"demo"},"nested":{"access_token":"token"}}`))
	if err != nil {
		t.Fatalf("sanitizePayload() error = %v", err)
	}
	if string(got) != `{"account":{"username":"demo"},"nested":{}}` {
		t.Fatalf("sanitizePayload() = %s, want sensitive fields removed", got)
	}
}

func TestSanitizePayloadRejectsInvalidJSON(t *testing.T) {
	if _, err := sanitizePayload([]byte(`not-json`)); err == nil {
		t.Fatal("sanitizePayload() error = nil, want error")
	}
}
