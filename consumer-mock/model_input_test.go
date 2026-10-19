package main

import (
	"testing"
)

func TestSelectModelInputFromBankEvent(t *testing.T) {
	input, ok, err := selectModelInput([]byte(`{
      "event_type":"TransferCompleted",
      "payload": {
        "transfer": {
          "ID":"TX001",
          "Type":"transfer",
          "Source":{"AccountNo":"ACC001"},
          "Destination":{"AccountNo":"ACC002"},
          "Amount":50000000,
          "Channel":"mobile",
          "CreatedAt":"2026-10-17T00:50:00+07:00",
          "DeviceID":"DEVICE01",
          "Platform":"android",
          "Authentication":{"Method":"email_otp","Verified":true}
        }
      }
    }`))
	if err != nil || !ok {
		t.Fatalf("selectModelInput() = %#v/%v/%v, want selected input", input, ok, err)
	}
	if input.TransactionID != "TX001" || input.SourceAccount != "ACC001" || input.DestinationAccount != "ACC002" || input.Amount != 50000000 || input.TransactionType != "TRANSFER" || input.Channel != "MOBILE" || input.AuthenticationMethod != "OTP" || !input.AuthenticationVerified || input.DeviceID != "DEVICE01" || input.Platform != "ANDROID" {
		t.Fatalf("selected model input = %#v", input)
	}
}

func TestSelectModelInputIgnoresOtherEvents(t *testing.T) {
	_, ok, err := selectModelInput([]byte(`{"event_type":"TransferCreated","payload":{}}`))
	if err != nil || ok {
		t.Fatalf("selectModelInput() = selected=%v error=%v, want ignored", ok, err)
	}
}

func TestSelectModelInputUsesUnknownMetadata(t *testing.T) {
	input, ok, err := selectModelInput([]byte(`{"event_type":"TransferCompleted","payload":{"transfer":{"ID":"TX001","Type":"transfer","Source":{"AccountNo":"ACC001"},"Destination":{"AccountNo":"ACC002"},"Amount":1,"CreatedAt":"2026-10-17T00:50:00Z","Authentication":{"Verified":true}}}}`))
	if err != nil || !ok {
		t.Fatalf("selectModelInput() = %#v/%v/%v, want selected input", input, ok, err)
	}
	if input.DeviceID != "UNKNOWN" || input.Platform != "UNKNOWN" || input.AuthenticationMethod != "UNKNOWN" {
		t.Fatalf("unknown metadata = %#v", input)
	}
}
