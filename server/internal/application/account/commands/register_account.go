package commands

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const minPasswordLength = 8

type RegisterAccountCommand struct {
	Username string
	Email    string
	Password string
}

type RegisterAccountCommandHandler struct {
	writeRepo AccountRepository
	readRepo  AccountRepository
}

func NewRegisterAccountCommandHandler(
	writeRepo AccountRepository,
	readRepo AccountRepository,
) *RegisterAccountCommandHandler {
	return &RegisterAccountCommandHandler{writeRepo: writeRepo, readRepo: readRepo}
}

func (h *RegisterAccountCommandHandler) Handle(ctx context.Context, cmd RegisterAccountCommand) (*accountdomain.UserAccount, error) {
	cmd.Email = strings.TrimSpace(cmd.Email)
	cmd.Username = strings.TrimSpace(cmd.Username)

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

	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	customerID := newID()
	accountID := newID()
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
				FullName: cmd.Username,
				Email:    cmd.Email,
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

	if err := h.writeRepo.Save(ctx, acc); err != nil {
		return nil, err
	}

	return acc, nil
}
