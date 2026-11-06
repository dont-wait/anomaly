package commands

import (
	"context"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// SubmitKYCCommand records manual KYC evidence uploaded by the customer.
// The session stays pending until an administrator reviews the documents.
type SubmitKYCCommand struct {
	AccountID      string
	IdCardFrontUrl string
	IdCardBackUrl  string
	LiveVideoUrl   string
	Media          *accountdomain.KYCMedia
}

type SubmitKYCCommandHandler struct {
	repo AccountRepository
}

func NewSubmitKYCCommandHandler(repo AccountRepository) *SubmitKYCCommandHandler {
	return &SubmitKYCCommandHandler{repo: repo}
}

func (h *SubmitKYCCommandHandler) Handle(ctx context.Context, cmd SubmitKYCCommand) (*accountdomain.UserAccount, error) {
	if err := validateKYCMediaKeys(cmd.AccountID, cmd.IdCardFrontUrl, cmd.IdCardBackUrl, cmd.LiveVideoUrl, cmd.Media); err != nil {
		return nil, err
	}

	acc, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if acc == nil || acc.Customer == nil {
		return nil, accountdomain.ErrAccountNotFound
	}
	// Submission is idempotent: never create a second pending attempt for the
	// same evidence while the first one waits for review.
	if acc.IsVerified() || acc.IsKYCUnderReview() {
		return acc, nil
	}

	now := time.Now().UTC()
	session := &accountdomain.KYCSession{
		Id:           newID(),
		CustomerId:   acc.CustomerId,
		AttemptNo:    len(acc.KYCSessions) + 1,
		Status:       accountdomain.KYCSessionStatusPending,
		IdentityData: declaredIdentity(acc),
		Media: accountdomain.KYCMedia{
			IdentityFront: accountdomain.MediaObject{StorageKey: cmd.IdCardFrontUrl},
			IdentityBack:  accountdomain.MediaObject{StorageKey: cmd.IdCardBackUrl},
			LivenessVideo: accountdomain.LivenessVideo{
				MediaObject: accountdomain.MediaObject{StorageKey: cmd.LiveVideoUrl},
			},
		},
		Verification: accountdomain.KYCVerification{
			OCRStatus:       accountdomain.VerificationStatusNotRun,
			LivenessStatus:  accountdomain.VerificationStatusNotRun,
			FaceMatchStatus: accountdomain.VerificationStatusNotRun,
			Provider:        "anomaly-manual-review",
		},
		StartedAt:   now,
		CompletedAt: &now,
		CreatedAt:   now,
	}
	if cmd.Media != nil {
		session.Media = *cmd.Media
	}

	acc.KYCSessions = append(acc.KYCSessions, session)
	acc.Customer.KYCStatus = accountdomain.KYCStatusPending
	acc.Customer.UpdatedAt = now
	acc.Version++
	acc.UpdatedAt = now

	if err := h.repo.Save(ctx, acc); err != nil {
		return nil, err
	}
	return acc, nil
}

// declaredIdentity snapshots the registration data so reviewers can compare it
// with the documents even when no OCR model runs.
func declaredIdentity(acc *accountdomain.UserAccount) accountdomain.KYCIdentityData {
	customer := acc.Customer
	identity := accountdomain.KYCIdentityData{
		Type:             customer.Identity.Type,
		Number:           customer.Identity.Number,
		FullName:         customer.Profile.FullName,
		DateOfBirth:      customer.Profile.DateOfBirth,
		IssuedDate:       customer.Identity.IssuedDate,
		IssuedPlace:      customer.Identity.IssuedPlace,
		PermanentAddress: customer.Identity.PermanentAddress,
	}
	return identity
}

func validateKYCMediaKeys(accountID, front, back, video string, media *accountdomain.KYCMedia) error {
	if strings.TrimSpace(accountID) == "" || front == "" || back == "" || video == "" {
		return accountdomain.ErrInvalidVerifyPayload
	}
	prefix := "kyc/" + accountID + "/"
	if !strings.HasPrefix(front, prefix+"id-card-front/") ||
		!strings.HasPrefix(back, prefix+"id-card-back/") ||
		!strings.HasPrefix(video, prefix+"live-video/") {
		return accountdomain.ErrInvalidVerifyPayload
	}
	if media != nil && (media.IdentityFront.StorageKey != front || media.IdentityBack.StorageKey != back || media.LivenessVideo.StorageKey != video) {
		return accountdomain.ErrInvalidVerifyPayload
	}
	return nil
}
