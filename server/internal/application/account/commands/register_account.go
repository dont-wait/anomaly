package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const minPasswordLength = 8

var idempotencyKeyRegex = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

var cccdRegex = regexp.MustCompile(`^\d{12}$`)

type RegisterAccountCommand struct {
	IdempotencyKey string
	Username       string
	CCCDNumber     string
	CCCDIssuedDate time.Time
	DOB            time.Time
	Email          string
	Password       string
}

type RegisterAccountCommandHandler struct {
	writeRepo AccountCreator
	readRepo  AccountRepository
}

func NewRegisterAccountCommandHandler(
	writeRepo AccountCreator,
	readRepo AccountRepository,
) *RegisterAccountCommandHandler {
	return &RegisterAccountCommandHandler{writeRepo: writeRepo, readRepo: readRepo}
}

func (h *RegisterAccountCommandHandler) Handle(ctx context.Context, cmd RegisterAccountCommand) (*accountdomain.UserAccount, error) {
	cmd.Email = strings.TrimSpace(cmd.Email)
	cmd.Username = strings.TrimSpace(cmd.Username)
	cmd.CCCDNumber = strings.TrimSpace(cmd.CCCDNumber)

	parsedEmail, err := mail.ParseAddress(cmd.Email)
	if err != nil {
		return nil, accountdomain.ErrInvalidEmail
	}
	cmd.Email = parsedEmail.Address

	if len(cmd.Password) < minPasswordLength {
		return nil, accountdomain.ErrWeakPassword
	}

	if cmd.Username == "" {
		return nil, accountdomain.ErrInvalidUsername
	}

	if !cccdRegex.MatchString(cmd.CCCDNumber) {
		return nil, accountdomain.ErrInvalidCCCD
	}

	if cmd.DOB.IsZero() || cmd.CCCDIssuedDate.IsZero() {
		return nil, accountdomain.ErrInvalidDate
	}
	if cmd.DOB.After(time.Now()) || cmd.CCCDIssuedDate.After(time.Now()) {
		return nil, accountdomain.ErrInvalidDate
	}
	if cmd.CCCDIssuedDate.Before(cmd.DOB) {
		return nil, accountdomain.ErrInvalidDate
	}

	accountID := newID()
	if cmd.IdempotencyKey != "" {
		if !idempotencyKeyRegex.MatchString(cmd.IdempotencyKey) {
			return nil, accountdomain.ErrInvalidIdempotencyKey
		}
		// The database primary key persists the attempt across processes/restarts.
		sum := sha256.Sum256([]byte("registration:" + strings.ToLower(cmd.IdempotencyKey)))
		accountID = hex.EncodeToString(sum[:16])
		if existing, err := h.replay(ctx, accountID, cmd); existing != nil || err != nil {
			return existing, err
		}
	}

	if existing, err := h.readRepo.FindByEmail(ctx, cmd.Email); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, accountdomain.ErrUserAlreadyExists
	}

	if existing, err := h.readRepo.FindByUsername(ctx, cmd.Username); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, accountdomain.ErrUserAlreadyExists
	}

	if existing, err := h.readRepo.FindByCCCDNumber(ctx, cmd.CCCDNumber); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, accountdomain.ErrUserAlreadyExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	customerID := newID()
	acc := &accountdomain.UserAccount{
		Id:           accountID,
		AccountNo:    fmt.Sprintf("ACC-%s", strings.ToUpper(accountID)),
		CustomerId:   customerID,
		Username:     cmd.Username,
		Email:        cmd.Email,
		PasswordHash: string(hash),
		Type:         accountdomain.AccountTypePayment,
		Currency:     accountdomain.CurrencyVND,
		Balance:      accountdomain.Balance{Current: 0},
		Status:       accountdomain.AccountStatusActive,
		Version:      1,
		OpenedAt:     now,
		CreatedAt:    now,
		UpdatedAt:    now,
		Customer: &accountdomain.Customer{
			Id:           customerID,
			CustomerCode: fmt.Sprintf("CUS-%s", strings.ToUpper(customerID)),
			Profile: accountdomain.CustomerProfile{
				FullName:    cmd.Username,
				DateOfBirth: &cmd.DOB,
				Email:       cmd.Email,
			},
			Identity: accountdomain.CustomerIdentity{
				Type:       "cccd",
				Number:     cmd.CCCDNumber,
				IssuedDate: &cmd.CCCDIssuedDate,
			},
			KYCStatus: accountdomain.KYCStatusNotStarted,
			CreditProfile: accountdomain.CreditProfile{
				UpdatedAt: now,
			},
			Status:    accountdomain.CustomerStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	if err := h.writeRepo.Create(ctx, acc); err != nil {
		if cmd.IdempotencyKey != "" && errors.Is(err, accountdomain.ErrUserAlreadyExists) {
			if existing, replayErr := h.replay(ctx, accountID, cmd); existing != nil || replayErr != nil {
				return existing, replayErr
			}
		}
		return nil, err
	}

	return acc, nil
}

// Replays only the matching registration, never another account's private state.
func (h *RegisterAccountCommandHandler) replay(ctx context.Context, id string, cmd RegisterAccountCommand) (*accountdomain.UserAccount, error) {
	acc, err := h.readRepo.FindByID(ctx, id)
	if err != nil || acc == nil {
		return acc, err
	}
	if acc.Customer == nil {
		return nil, accountdomain.ErrRegistrationPending
	}
	customer := acc.Customer
	if acc.Username != cmd.Username || acc.Email != cmd.Email || customer.Identity.Number != cmd.CCCDNumber ||
		customer.Profile.DateOfBirth == nil || !customer.Profile.DateOfBirth.Equal(cmd.DOB) ||
		customer.Identity.IssuedDate == nil || !customer.Identity.IssuedDate.Equal(cmd.CCCDIssuedDate) ||
		bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(cmd.Password)) != nil {
		return nil, accountdomain.ErrIdempotencyConflict
	}
	// Registration returns the original initial state, even after later onboarding.
	result := *acc
	profile := *customer
	profile.KYCStatus = accountdomain.KYCStatusNotStarted
	profile.VerifiedKYCSessionId = ""
	result.Customer = &profile
	result.KYCSessions = nil
	result.Balance = accountdomain.Balance{}
	return &result, nil
}
