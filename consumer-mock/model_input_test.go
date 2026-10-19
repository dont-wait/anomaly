package main

import (
	"testing"
)

func TestSelectModelInputFromBankEvent(t *testing.T) {
	input, ok, err := selectModelInput([]byte(`{
      "event_type":"TransferCompleted",
      "model_features": {
        "avg_amount_7d": 12500000,
        "tx_count_1h": 3,
        "tx_count_24h": 12,
        "last_tx_at": "2026-10-16T00:50:00Z",
        "new_receiver": true,
        "new_device": false
      },
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
	if input.TransactionID != "TX001" || input.SourceAccount != "ACC001" || input.DestinationAccount != "ACC002" || input.Amount != 50000000 || input.TransactionType != "TRANSFER" || input.Channel != "MOBILE" || input.AuthenticationMethod != "OTP" || !input.AuthenticationVerified || input.DeviceID != "DEVICE01" || input.Platform != "ANDROID" || input.AvgAmount7d != 12500000 || input.TxCount1h != 3 || input.TxCount24h != 12 || !input.NewReceiver || input.NewDevice {
		t.Fatalf("selected model input = %#v", input)
	}
}

func TestSelectModelInputIgnoresOtherEvents(t *testing.T) {
	_, ok, err := selectModelInput([]byte(`{"event_type":"TransferCreated","payload":{}}`))
	if err != nil || ok {
		t.Fatalf("selectModelInput() = selected=%v error=%v, want ignored", ok, err)
	}
}

func TestSelectModelInputRequiresProducerFeatures(t *testing.T) {
	_, ok, err := selectModelInput([]byte(`{"event_type":"TransferCompleted","payload":{}}`))
	if err == nil || !ok {
		t.Fatalf("selectModelInput() = selected=%v error=%v, want missing feature error", ok, err)
	}
}

func TestSelectModelInputUsesUnknownMetadata(t *testing.T) {
	input, ok, err := selectModelInput([]byte(`{"event_type":"TransferCompleted","model_features":{"avg_amount_7d":0,"tx_count_1h":0,"tx_count_24h":0,"last_tx_at":null,"new_receiver":false,"new_device":false},"payload":{"transfer":{"ID":"TX001","Type":"transfer","Source":{"AccountNo":"ACC001"},"Destination":{"AccountNo":"ACC002"},"Amount":1,"CreatedAt":"2026-10-17T00:50:00Z","Authentication":{"Verified":true}}}}`))
	if err != nil || !ok {
		t.Fatalf("selectModelInput() = %#v/%v/%v, want selected input", input, ok, err)
	}
	if input.DeviceID != "UNKNOWN" || input.Platform != "UNKNOWN" || input.AuthenticationMethod != "UNKNOWN" {
		t.Fatalf("unknown metadata = %#v", input)
	}
}
