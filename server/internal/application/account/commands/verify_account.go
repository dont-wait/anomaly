package commands

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type VerifyAccountCommand struct {
	AccountID      string
	IdCardFrontUrl string
	IdCardBackUrl  string
	LiveVideoUrl   string
	Evidence       *accountdomain.KYCEvidence
	MatchScore     float64
	LivenessScore  float64
	Media          *accountdomain.KYCMedia
}

type VerifyAccountCommandHandler struct {
	repo AccountRepository
}

func NewVerifyAccountCommandHandler(repo AccountRepository) *VerifyAccountCommandHandler {
	return &VerifyAccountCommandHandler{repo: repo}
}

func (h *VerifyAccountCommandHandler) Handle(ctx context.Context, cmd VerifyAccountCommand) (*accountdomain.UserAccount, error) {
	if cmd.IdCardFrontUrl == "" || cmd.IdCardBackUrl == "" || cmd.LiveVideoUrl == "" {
		return nil, accountdomain.ErrInvalidVerifyPayload
	}
	prefix := "kyc/" + cmd.AccountID + "/"
	if !strings.HasPrefix(cmd.IdCardFrontUrl, prefix+"id-card-front/") ||
		!strings.HasPrefix(cmd.IdCardBackUrl, prefix+"id-card-back/") ||
		!strings.HasPrefix(cmd.LiveVideoUrl, prefix+"live-video/") {
		return nil, accountdomain.ErrInvalidVerifyPayload
	}
	if cmd.Media != nil && (cmd.Media.IdentityFront.StorageKey != cmd.IdCardFrontUrl || cmd.Media.IdentityBack.StorageKey != cmd.IdCardBackUrl || cmd.Media.LivenessVideo.StorageKey != cmd.LiveVideoUrl) {
		return nil, accountdomain.ErrInvalidVerifyPayload
	}

	acc, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, accountdomain.ErrAccountNotFound
	}

	if acc.Customer == nil {
		return nil, accountdomain.ErrAccountNotFound
	}
	// Verification is idempotent: keep the original verified session as the
	// official KYC record when the command is retried.
	if acc.IsVerified() {
		return acc, nil
	}
	if cmd.Evidence == nil || cmd.Evidence.MatchingStatus != "passed" ||
		cmd.Evidence.LivenessStatus != "passed" || !cmd.Evidence.ChallengePassed ||
		!cmd.Evidence.SubjectConsistencyPassed || math.IsNaN(cmd.MatchScore) || math.IsInf(cmd.MatchScore, 0) ||
		cmd.MatchScore < 0 || cmd.MatchScore > 1 || math.IsNaN(cmd.LivenessScore) ||
		math.IsInf(cmd.LivenessScore, 0) || cmd.LivenessScore < 0 || cmd.LivenessScore > 1 {
		return nil, accountdomain.ErrInvalidVerifyPayload
	}

	now := time.Now().UTC()
	session := &accountdomain.KYCSession{
		Id:         newID(),
		CustomerId: acc.CustomerId,
		AttemptNo:  len(acc.KYCSessions) + 1,
		Status:     accountdomain.KYCSessionStatusVerified,
		Media: accountdomain.KYCMedia{
			IdentityFront: accountdomain.MediaObject{StorageKey: cmd.IdCardFrontUrl},
			IdentityBack:  accountdomain.MediaObject{StorageKey: cmd.IdCardBackUrl},
			LivenessVideo: accountdomain.LivenessVideo{
				MediaObject: accountdomain.MediaObject{StorageKey: cmd.LiveVideoUrl},
			},
		},
		Verification: accountdomain.KYCVerification{
			OCRStatus:           accountdomain.VerificationStatusNotRun,
			LivenessStatus:      accountdomain.VerificationStatusPassed,
			FaceMatchStatus:     accountdomain.VerificationStatusPassed,
			FaceMatchScore:      strconv.FormatFloat(cmd.MatchScore, 'f', -1, 64),
			LivenessScore:       cmd.LivenessScore,
			Provider:            "anomaly-real-vision",
			ProviderReferenceId: cmd.Evidence.RequestID,
			Evidence:            cmd.Evidence,
		},
		StartedAt:   now,
		CompletedAt: &now,
		CreatedAt:   now,
	}
	if cmd.Media != nil {
		session.Media = *cmd.Media
	}
	acc.KYCSessions = append(acc.KYCSessions, session)
	acc.Customer.VerifiedKYCSessionId = session.Id
	acc.Customer.KYCStatus = accountdomain.KYCStatusVerified
	acc.Customer.UpdatedAt = now
	acc.Version++
	acc.UpdatedAt = now

	if err := h.repo.Save(ctx, acc); err != nil {
		return nil, err
	}

	return acc, nil
}
