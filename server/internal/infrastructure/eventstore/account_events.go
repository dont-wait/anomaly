package eventstore

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const (
	EventAccountCreated  = "AccountCreated"
	EventAccountVerified = "AccountVerified"
	EventAccountWithdraw = "AccountWithdraw"
)

type accountCreatedPayload struct {
	Account      *accountdomain.UserAccount `json:"account"`
	Id           string                     `json:"id,omitempty"`
	Username     string                     `json:"username,omitempty"`
	Email        string                     `json:"email,omitempty"`
	PasswordHash string                     `json:"passwordHash,omitempty"`
}

type accountVerifiedPayload struct {
	Session              *accountdomain.KYCSession `json:"session"`
	VerifiedKYCSessionId string                    `json:"verifiedKycSessionId"`
	Version              *int64                    `json:"version"`
	UpdatedAt            *time.Time                `json:"updatedAt"`
	IdCardFrontUrl       string                    `json:"idCardFrontUrl,omitempty"`
	IdCardBackUrl        string                    `json:"idCardBackUrl,omitempty"`
	LiveVideoUrl         string                    `json:"liveVideoUrl,omitempty"`
}

type accountWithdrawPayload struct {
	Amount    int64      `json:"amount"`
	Version   *int64     `json:"version"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

type replayEventMetadata struct {
	Revision  uint64
	CreatedAt time.Time
}

// streamName sinh tên stream cho 1 account cụ thể, dựa vào ID.
func streamName(accountID string) string {
	return fmt.Sprintf("account-%s", accountID)
}

// applyEvent áp 1 sự kiện đã đọc được lên 1 UserAccount, để dựng lại
// đúng trạng thái hiện tại từ toàn bộ lịch sử (replay).
func applyEvent(
	acc *accountdomain.UserAccount,
	eventType string,
	metadata replayEventMetadata,
	decode func(v any) error,
) error {
	switch eventType {
	case EventAccountCreated:
		var p accountCreatedPayload
		if err := decode(&p); err != nil {
			return err
		}
		*acc = upcastAccountCreated(p, metadata)

	case EventAccountVerified:
		var p accountVerifiedPayload
		if err := decode(&p); err != nil {
			return err
		}
		if acc.Customer == nil {
			return fmt.Errorf("account %s has no customer", acc.Id)
		}
		session, verifiedSessionID, version, updatedAt := upcastAccountVerified(acc, p, metadata)
		acc.KYCSessions = append(acc.KYCSessions, session)
		acc.Customer.VerifiedKYCSessionId = verifiedSessionID
		acc.Customer.KYCStatus = accountdomain.KYCStatusVerified
		acc.Customer.UpdatedAt = updatedAt
		acc.Version = version
		acc.UpdatedAt = updatedAt

	case EventAccountWithdraw:
		var p accountWithdrawPayload
		if err := decode(&p); err != nil {
			return err
		}
		acc.Balance.Current -= p.Amount
		acc.Version, acc.UpdatedAt = upcastVersionAndTime(p.Version, p.UpdatedAt, metadata)
	}

	return nil
}

func upcastAccountCreated(p accountCreatedPayload, metadata replayEventMetadata) accountdomain.UserAccount {
	if p.Account != nil {
		return *p.Account
	}

	customerID := legacyObjectID("customer", p.Id)
	return accountdomain.UserAccount{
		Id:           p.Id,
		AccountNo:    fmt.Sprintf("ACC-%s", strings.ToUpper(p.Id)),
		CustomerId:   customerID,
		Username:     p.Username,
		Email:        p.Email,
		PasswordHash: p.PasswordHash,
		Type:         accountdomain.AccountTypePayment,
		Currency:     accountdomain.CurrencyVND,
		Status:       accountdomain.AccountStatusActive,
		Version:      legacyVersion(metadata.Revision),
		OpenedAt:     metadata.CreatedAt,
		CreatedAt:    metadata.CreatedAt,
		UpdatedAt:    metadata.CreatedAt,
		Customer: &accountdomain.Customer{
			Id:           customerID,
			CustomerCode: fmt.Sprintf("CUS-%s", strings.ToUpper(customerID)),
			Profile: accountdomain.CustomerProfile{
				FullName: p.Username,
				Email:    p.Email,
			},
			KYCStatus: accountdomain.KYCStatusNotStarted,
			CreditProfile: accountdomain.CreditProfile{
				UpdatedAt: metadata.CreatedAt,
			},
			Status:    accountdomain.CustomerStatusActive,
			CreatedAt: metadata.CreatedAt,
			UpdatedAt: metadata.CreatedAt,
		},
	}
}

func upcastAccountVerified(
	acc *accountdomain.UserAccount,
	p accountVerifiedPayload,
	metadata replayEventMetadata,
) (*accountdomain.KYCSession, string, int64, time.Time) {
	if p.Session != nil {
		version, updatedAt := upcastVersionAndTime(p.Version, p.UpdatedAt, metadata)
		return p.Session, p.VerifiedKYCSessionId, version, updatedAt
	}

	sessionID := legacyObjectID("kyc", acc.Id, fmt.Sprint(metadata.Revision))
	completedAt := metadata.CreatedAt
	session := &accountdomain.KYCSession{
		Id:         sessionID,
		CustomerId: acc.CustomerId,
		AttemptNo:  len(acc.KYCSessions) + 1,
		Status:     accountdomain.KYCSessionStatusVerified,
		Media: accountdomain.KYCMedia{
			IdentityFront: accountdomain.MediaObject{StorageKey: p.IdCardFrontUrl},
			IdentityBack:  accountdomain.MediaObject{StorageKey: p.IdCardBackUrl},
			LivenessVideo: accountdomain.LivenessVideo{
				MediaObject: accountdomain.MediaObject{StorageKey: p.LiveVideoUrl},
			},
		},
		Verification: accountdomain.KYCVerification{
			OCRStatus:       accountdomain.VerificationStatusNotRun,
			LivenessStatus:  accountdomain.VerificationStatusNotRun,
			FaceMatchStatus: accountdomain.VerificationStatusNotRun,
		},
		StartedAt:   metadata.CreatedAt,
		CompletedAt: &completedAt,
		CreatedAt:   metadata.CreatedAt,
	}
	return session, sessionID, legacyVersion(metadata.Revision), metadata.CreatedAt
}

func upcastVersionAndTime(
	version *int64,
	updatedAt *time.Time,
	metadata replayEventMetadata,
) (int64, time.Time) {
	if version != nil && updatedAt != nil {
		return *version, *updatedAt
	}
	return legacyVersion(metadata.Revision), metadata.CreatedAt
}

func legacyVersion(revision uint64) int64 {
	return int64(revision) + 1
}

func legacyObjectID(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return fmt.Sprintf("%x", digest[:12])
}
