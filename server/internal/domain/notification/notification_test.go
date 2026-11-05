package notification

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNotificationIDDeterministicAndScoped(t *testing.T) {
	a := NotificationID("evt-1", TypeTransferSent, "user-1")
	if a != NotificationID("evt-1", TypeTransferSent, "user-1") {
		t.Fatal("id must be deterministic")
	}
	if a == NotificationID("evt-1", TypeTransferReceived, "user-1") {
		t.Fatal("id must differ by type")
	}
	if a == NotificationID("evt-1", TypeTransferSent, "user-2") {
		t.Fatal("id must differ by recipient")
	}
}

func validNotification() Notification {
	now := time.Now().UTC()
	return Notification{
		ID: "id", Audience: AudienceCustomer, RecipientID: "user", Type: TypeSystemTest,
		Category: CategorySystem, Severity: SeverityInfo, Title: "t", SourceEventID: "src",
		OccurredAt: now, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
}

func TestValidate(t *testing.T) {
	if err := validNotification().Validate(); err != nil {
		t.Fatalf("valid notification rejected: %v", err)
	}
	tooMany := map[string]string{}
	for i := 0; i <= MaxDataKeys; i++ {
		tooMany["k"+strconv.Itoa(i)] = "v"
	}
	cases := map[string]func(*Notification){
		"missing id":        func(n *Notification) { n.ID = "" },
		"missing recipient": func(n *Notification) { n.RecipientID = " " },
		"missing source":    func(n *Notification) { n.SourceEventID = "" },
		"missing title":     func(n *Notification) { n.Title = "" },
		"bad audience":      func(n *Notification) { n.Audience = "X" },
		"bad severity":      func(n *Notification) { n.Severity = "X" },
		"too many keys":     func(n *Notification) { n.Data = tooMany },
		"long value":        func(n *Notification) { n.Data = map[string]string{"k": strings.Repeat("a", MaxDataValueRunes+1)} },
	}
	for name, mutate := range cases {
		n := validNotification()
		mutate(&n)
		if err := n.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: expected ErrInvalid, got %v", name, err)
		}
	}
}
