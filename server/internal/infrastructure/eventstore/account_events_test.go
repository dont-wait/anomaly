package eventstore

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestApplyEventUpcastsLegacyAccountEvents(t *testing.T) {
	createdAt := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	verifiedAt := createdAt.Add(time.Minute)
	withdrawnAt := verifiedAt.Add(time.Minute)
	events := []struct {
		eventType string
		data      string
		metadata  replayEventMetadata
	}{
		{
			eventType: EventAccountCreated,
			data:      `{"id":"0123456789abcdef0123456789abcdef","username":"legacy","email":"legacy@example.com","passwordHash":"hash"}`,
			metadata:  replayEventMetadata{Revision: 0, CreatedAt: createdAt},
		},
		{
			eventType: EventAccountVerified,
			data:      `{"idCardFrontUrl":"front.jpg","idCardBackUrl":"back.jpg","liveVideoUrl":"live.mp4"}`,
			metadata:  replayEventMetadata{Revision: 1, CreatedAt: verifiedAt},
		},
		{
			eventType: EventAccountWithdraw,
			data:      `{"amount":250}`,
			metadata:  replayEventMetadata{Revision: 2, CreatedAt: withdrawnAt},
		},
	}

	replay := func() *accountdomain.UserAccount {
		t.Helper()
		account := &accountdomain.UserAccount{}
		for _, event := range events {
			decode := func(value any) error {
				return json.Unmarshal([]byte(event.data), value)
			}
			if err := applyEvent(account, event.eventType, event.metadata, decode); err != nil {
				t.Fatalf("applyEvent(%s) error = %v", event.eventType, err)
			}
		}
		return account
	}

	account := replay()
	if account.Id != "0123456789abcdef0123456789abcdef" ||
		account.Username != "legacy" ||
		account.Email != "legacy@example.com" ||
		account.PasswordHash != "hash" {
		t.Fatalf("legacy AccountCreated fields were not preserved: %#v", account)
	}
	if account.Customer == nil {
		t.Fatal("legacy account customer = nil")
	}
	if account.Customer.KYCStatus != accountdomain.KYCStatusVerified {
		t.Errorf("customer KYC status = %q, want %q", account.Customer.KYCStatus, accountdomain.KYCStatusVerified)
	}
	if len(account.KYCSessions) != 1 {
		t.Fatalf("KYC session count = %d, want 1", len(account.KYCSessions))
	}
	session := account.KYCSessions[0]
	if session.Media.IdentityFront.StorageKey != "front.jpg" ||
		session.Media.IdentityBack.StorageKey != "back.jpg" ||
		session.Media.LivenessVideo.StorageKey != "live.mp4" {
		t.Errorf("legacy verification URLs were not preserved: %#v", session.Media)
	}
	if account.Balance.Current != -250 {
		t.Errorf("balance = %d, want -250", account.Balance.Current)
	}
	if account.Version != 3 || !account.UpdatedAt.Equal(withdrawnAt) {
		t.Errorf("version/time = %d/%v, want 3/%v", account.Version, account.UpdatedAt, withdrawnAt)
	}
	if secondReplay := replay(); !reflect.DeepEqual(account, secondReplay) {
		t.Errorf("legacy replay is not deterministic:\nfirst:  %#v\nsecond: %#v", account, secondReplay)
	}
}

func TestApplyEventSupportsCurrentPayloadAfterLegacyEvents(t *testing.T) {
	createdAt := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	account := &accountdomain.UserAccount{}
	decodeLegacyCreated := func(value any) error {
		return json.Unmarshal([]byte(`{"id":"0123456789abcdef0123456789abcdef","username":"legacy","email":"legacy@example.com","passwordHash":"hash"}`), value)
	}
	if err := applyEvent(account, EventAccountCreated, replayEventMetadata{Revision: 0, CreatedAt: createdAt}, decodeLegacyCreated); err != nil {
		t.Fatalf("applyEvent(AccountCreated) error = %v", err)
	}

	updatedAt := createdAt.Add(time.Minute)
	version := int64(2)
	session := &accountdomain.KYCSession{Id: "507f1f77bcf86cd799439011", CustomerId: account.CustomerId}
	payload, err := json.Marshal(accountVerifiedPayload{
		Session:              session,
		VerifiedKYCSessionId: session.Id,
		Version:              &version,
		UpdatedAt:            &updatedAt,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	decodeCurrentVerified := func(value any) error { return json.Unmarshal(payload, value) }
	if err := applyEvent(account, EventAccountVerified, replayEventMetadata{Revision: 1}, decodeCurrentVerified); err != nil {
		t.Fatalf("applyEvent(AccountVerified) error = %v", err)
	}

	if len(account.KYCSessions) != 1 || account.KYCSessions[0].Id != session.Id {
		t.Errorf("current KYC session was not applied: %#v", account.KYCSessions)
	}
	if account.Version != version || !account.UpdatedAt.Equal(updatedAt) {
		t.Errorf("current version/time = %d/%v, want %d/%v", account.Version, account.UpdatedAt, version, updatedAt)
	}

	withdrawnAt := updatedAt.Add(time.Minute)
	withdrawVersion := int64(3)
	payload, err = json.Marshal(accountWithdrawPayload{
		Amount:    25,
		Version:   &withdrawVersion,
		UpdatedAt: &withdrawnAt,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	decodeCurrentWithdraw := func(value any) error { return json.Unmarshal(payload, value) }
	if err := applyEvent(account, EventAccountWithdraw, replayEventMetadata{Revision: 2}, decodeCurrentWithdraw); err != nil {
		t.Fatalf("applyEvent(AccountWithdraw) error = %v", err)
	}
	if account.Balance.Current != -25 || account.Version != withdrawVersion || !account.UpdatedAt.Equal(withdrawnAt) {
		t.Errorf(
			"current withdrawal state = %d/%d/%v, want -25/%d/%v",
			account.Balance.Current,
			account.Version,
			account.UpdatedAt,
			withdrawVersion,
			withdrawnAt,
		)
	}
}

func TestApplyEventPreservesCurrentAccountCreatedPayload(t *testing.T) {
	want := &accountdomain.UserAccount{
		Id:         "507f1f77bcf86cd799439011",
		CustomerId: "507f191e810c19729de860ea",
		Username:   "current",
		Version:    7,
		Customer:   &accountdomain.Customer{Id: "507f191e810c19729de860ea"},
	}
	payload, err := json.Marshal(accountCreatedPayload{Account: want})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	decode := func(value any) error { return json.Unmarshal(payload, value) }
	got := &accountdomain.UserAccount{}
	if err := applyEvent(got, EventAccountCreated, replayEventMetadata{Revision: 99}, decode); err != nil {
		t.Fatalf("applyEvent(AccountCreated) error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("current account = %#v, want %#v", got, want)
	}
}
