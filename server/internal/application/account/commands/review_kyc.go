package commands

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const maxKYCRejectionReason = 500

// ReviewKYCCommand is the administrator decision for a submitted KYC attempt.
type ReviewKYCCommand struct {
	AdminID   string
	AccountID string
	SessionID string
	Decision  string
	Reason    string
}

type ReviewKYCCommandHandler struct {
	repo AccountRepository
}

func NewReviewKYCCommandHandler(repo AccountRepository) *ReviewKYCCommandHandler {
	return &ReviewKYCCommandHandler{repo: repo}
}

// Decide approves or rejects the pending session. Approval promotes the session
// to the official verified KYC record; rejection lets the customer submit again.
func (h *ReviewKYCCommandHandler) Decide(ctx context.Context, cmd ReviewKYCCommand) (*accountdomain.UserAccount, error) {
	if cmd.AdminID == cmd.AccountID {
		return nil, accountdomain.ErrSelfKYCReview
	}
	reason := strings.TrimSpace(cmd.Reason)
	switch cmd.Decision {
	case accountdomain.KYCReviewApproved:
		reason = ""
	case accountdomain.KYCReviewRejected:
		if reason == "" || utf8.RuneCountInString(reason) > maxKYCRejectionReason {
			return nil, accountdomain.ErrInvalidKYCReview
		}
	default:
		return nil, accountdomain.ErrInvalidKYCReview
	}

	acc, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if acc == nil || acc.Customer == nil {
		return nil, accountdomain.ErrAccountNotFound
	}
	session := pendingSession(acc, cmd.SessionID)
	if session == nil {
		return nil, accountdomain.ErrKYCReviewClosed
	}

	now := time.Now().UTC()
	session.Review = &accountdomain.KYCReview{
		Decision:   cmd.Decision,
		Reason:     reason,
		ReviewedBy: cmd.AdminID,
		ReviewedAt: now,
	}
	if cmd.Decision == accountdomain.KYCReviewApproved {
		session.Status = accountdomain.KYCSessionStatusVerified
		acc.Customer.KYCStatus = accountdomain.KYCStatusVerified
		acc.Customer.VerifiedKYCSessionId = session.Id
	} else {
		session.Status = accountdomain.KYCSessionStatusRejected
		acc.Customer.KYCStatus = accountdomain.KYCStatusRejected
		acc.Customer.VerifiedKYCSessionId = ""
	}
	acc.Customer.UpdatedAt = now
	acc.Version++
	acc.UpdatedAt = now

	if err := h.repo.Save(ctx, acc); err != nil {
		return nil, err
	}
	return acc, nil
}

// pendingSession returns the session under review, defaulting to the newest one
// so an administrator can decide without tracking the ID.
func pendingSession(acc *accountdomain.UserAccount, sessionID string) *accountdomain.KYCSession {
	if sessionID != "" {
		for _, session := range acc.KYCSessions {
			if session != nil && session.Id == sessionID && session.Status == accountdomain.KYCSessionStatusPending {
				return session
			}
		}
		return nil
	}
	return acc.PendingKYCSession()
}
