package account

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestProfileUpdateRejectsEverythingExceptPhone(t *testing.T) {
	for _, body := range []string{`{"fullName":"x"}`, `{"address":{"line":"x"}}`, `{"issuedPlace":"x"}`, `{"permanentAddress":"x"}`, `{"identityNumber":"1"}`, `{"email":"a@b.c"}`} {
		decoder := json.NewDecoder(strings.NewReader(body))
		decoder.DisallowUnknownFields()
		var request profileUpdateRequest
		if err := decoder.Decode(&request); err == nil {
			t.Errorf("%s was accepted", body)
		}
	}
}

func verifiedAccount() *accountdomain.UserAccount {
	session := &accountdomain.KYCSession{Id: "s1", Status: accountdomain.KYCSessionStatusVerified, CreatedAt: time.Now()}
	session.Media.IdentityFront.StorageKey = "kyc/secret/front.jpg"
	session.Media.IdentityFront.MIMEType = "image/jpeg"
	session.Media.IdentityBack.StorageKey = "kyc/secret/back.jpg"
	session.Media.LivenessVideo.StorageKey = "kyc/secret/video.mp4"
	return &accountdomain.UserAccount{
		Id: "owner", AccountNo: "1234567890", Username: "user",
		Customer:    &accountdomain.Customer{VerifiedKYCSessionId: "s1", KYCStatus: accountdomain.KYCStatusVerified},
		KYCSessions: []*accountdomain.KYCSession{session},
	}
}

func TestProfileResponsesContainNoStorageKeys(t *testing.T) {
	account := verifiedAccount()
	account.Customer.Avatar = &accountdomain.ProfileMedia{ID: "a1", StorageKey: "profile/owner/secret.jpg", Status: "ACTIVE"}
	body, err := json.Marshal([]any{toProfileResponse(account), toAccountResponsePrivate(account), toProfileDocumentSlots(account), toAuthResponse(account, "t", time.Now())})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret", "idCardFrontUrl", "liveVideoUrl", "mediaUrl"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("response contains %q: %s", forbidden, body)
		}
	}
}

func TestDocumentSlotsExposePolicyAndReadOnlyIdentity(t *testing.T) {
	account := verifiedAccount()
	account.Customer.Documents = []accountdomain.ProfileMedia{
		{ID: "old", Type: "INCOME_PROOF", Status: "REPLACED"},
		{ID: "new", Type: "INCOME_PROOF", Status: "ACTIVE"},
	}
	slots := toProfileDocumentSlots(account)
	byType := map[string]ProfileDocumentSlot{}
	for _, slot := range slots {
		byType[slot.Type] = slot
	}
	if len(slots) != 9 {
		t.Fatalf("slots = %#v", slots)
	}
	front := byType["IDENTITY_FRONT"]
	if front.Replaceable || !front.Required || front.Document == nil || front.Document.MediaID != "kyc-identity-front" {
		t.Fatalf("identity slot = %#v", front)
	}
	income := byType["INCOME_PROOF"]
	if !income.Replaceable || income.Document == nil || income.Document.MediaID != "new" {
		t.Fatalf("income slot = %#v", income)
	}
	if address := byType["ADDRESS_PROOF"]; address.Document != nil || address.Required {
		t.Fatalf("empty slot = %#v", address)
	}
}

func TestProfileErrorsMapToSpecStatuses(t *testing.T) {
	for err, want := range map[error]int{
		accountdomain.ErrCurrentPassword:  http.StatusBadRequest,
		accountdomain.ErrTooManyAttempts:  http.StatusTooManyRequests,
		accountdomain.ErrConcurrentUpdate: http.StatusConflict,
		appotp.ErrInvalid:                 http.StatusBadRequest,
		appotp.ErrExpired:                 http.StatusBadRequest,
		appotp.ErrTooSoon:                 http.StatusTooManyRequests,
	} {
		if got := accountErrorStatus(err); got != want {
			t.Errorf("%v: status = %d, want %d", err, got, want)
		}
	}
}
