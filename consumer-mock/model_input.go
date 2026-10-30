package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const modelEventType = "TransferCreated"

type bankEventEnvelope struct {
	EventType     string          `json:"event_type"`
	EventID       string          `json:"event_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Payload       json.RawMessage `json:"payload"`
	ModelFeatures *modelFeatures  `json:"model_features"`
}

type modelFeatures struct {
	AvgAmount7d int64      `json:"avg_amount_7d"`
	TxCount1h   int64      `json:"tx_count_1h"`
	TxCount24h  int64      `json:"tx_count_24h"`
	LastTxAt    *time.Time `json:"last_tx_at"`
	NewReceiver bool       `json:"new_receiver"`
	NewDevice   bool       `json:"new_device"`
}

type bankRecord struct {
	Transfer json.RawMessage `json:"transfer"`
}

type transferSnapshot struct {
	ID     string `json:"ID"`
	Type   string `json:"Type"`
	Source struct {
		AccountNo string `json:"AccountNo"`
	} `json:"Source"`
	Destination struct {
		AccountNo string `json:"AccountNo"`
	} `json:"Destination"`
	Amount         int64     `json:"Amount"`
	Channel        string    `json:"Channel"`
	CreatedAt      time.Time `json:"CreatedAt"`
	DeviceID       string    `json:"DeviceID"`
	ClientIP       string    `json:"ClientIP"`
	Platform       string    `json:"Platform"`
	Authentication struct {
		Method   string `json:"Method"`
		Verified bool   `json:"Verified"`
	} `json:"Authentication"`
}

type modelInput struct {
	EventID                string     `json:"event_id"`
	TransactionID          string     `json:"transaction_id"`
	SourceAccount          string     `json:"source_account"`
	DestinationAccount     string     `json:"destination_account"`
	Amount                 int64      `json:"amount"`
	TransactionType        string     `json:"transaction_type"`
	Channel                string     `json:"channel"`
	AuthenticationMethod   string     `json:"authentication_method"`
	AuthenticationVerified bool       `json:"authentication_verified"`
	DeviceID               string     `json:"device_id"`
	IPAddress              string     `json:"ip_address"`
	Platform               string     `json:"platform"`
	CreatedAt              time.Time  `json:"created_at"`
	AvgAmount7d            int64      `json:"avg_amount_7d"`
	TxCount1h              int64      `json:"tx_count_1h"`
	TxCount24h             int64      `json:"tx_count_24h"`
	LastTxAt               *time.Time `json:"last_tx_at"`
	NewReceiver            bool       `json:"new_receiver"`
	NewDevice              bool       `json:"new_device"`
}

func selectModelInput(data []byte) (modelInput, bool, error) {
	var envelope bankEventEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return modelInput{}, false, nil
	}
	if envelope.EventType != modelEventType {
		return modelInput{}, false, nil
	}
	if strings.TrimSpace(envelope.EventID) == "" {
		return modelInput{}, true, fmt.Errorf("created event id is missing")
	}
	if envelope.ModelFeatures == nil {
		return modelInput{}, true, fmt.Errorf("created event model features are missing")
	}
	if len(envelope.Payload) == 0 {
		return modelInput{}, true, fmt.Errorf("created event payload is missing")
	}

	var record bankRecord
	if err := json.Unmarshal(envelope.Payload, &record); err != nil {
		return modelInput{}, true, fmt.Errorf("decode banking record: %w", err)
	}
	if len(record.Transfer) == 0 {
		return modelInput{}, true, fmt.Errorf("created transfer snapshot is missing")
	}

	var transfer transferSnapshot
	if err := json.Unmarshal(record.Transfer, &transfer); err != nil {
		return modelInput{}, true, fmt.Errorf("decode transfer snapshot: %w", err)
	}
	if strings.TrimSpace(transfer.ID) == "" || strings.TrimSpace(transfer.Source.AccountNo) == "" || strings.TrimSpace(transfer.Destination.AccountNo) == "" {
		return modelInput{}, true, fmt.Errorf("transfer snapshot is missing transaction or account numbers")
	}
	if transfer.Amount <= 0 || transfer.CreatedAt.IsZero() {
		return modelInput{}, true, fmt.Errorf("transfer snapshot has invalid amount or created_at")
	}

	occurredAt := envelope.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = transfer.CreatedAt
	}
	return modelInput{
		TransactionID:          transfer.ID,
		EventID:                envelope.EventID,
		SourceAccount:          transfer.Source.AccountNo,
		DestinationAccount:     transfer.Destination.AccountNo,
		Amount:                 transfer.Amount,
		TransactionType:        normalizeEnum(transfer.Type, "UNKNOWN"),
		Channel:                normalizeEnum(transfer.Channel, "UNKNOWN"),
		AuthenticationMethod:   normalizeAuthenticationMethod(transfer.Authentication.Method),
		AuthenticationVerified: transfer.Authentication.Verified,
		DeviceID:               normalizeValue(transfer.DeviceID),
		IPAddress:              normalizeValue(transfer.ClientIP),
		Platform:               normalizeEnum(transfer.Platform, "UNKNOWN"),
		CreatedAt:              occurredAt,
		AvgAmount7d:            envelope.ModelFeatures.AvgAmount7d,
		TxCount1h:              envelope.ModelFeatures.TxCount1h,
		TxCount24h:             envelope.ModelFeatures.TxCount24h,
		LastTxAt:               envelope.ModelFeatures.LastTxAt,
		NewReceiver:            envelope.ModelFeatures.NewReceiver,
		NewDevice:              envelope.ModelFeatures.NewDevice,
	}, true, nil
}

func normalizeAuthenticationMethod(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	if value == "EMAIL_OTP" || value == "OTP" {
		return "OTP"
	}
	if value == "" {
		return "UNKNOWN"
	}
	return value
}

func normalizeEnum(value, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func normalizeValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "UNKNOWN"
	}
	return value
}
