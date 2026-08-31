package commands

import (
	"context"
	"net/mail"
	"strings"

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
	if _, err := mail.ParseAddress(cmd.Email); err != nil {
		return nil, accountdomain.ErrInvalidEmail
	}
	if len(cmd.Password) < minPasswordLength {
		return nil, accountdomain.ErrWeakPassword
	}
	if strings.TrimSpace(cmd.Username) == "" {
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

	acc := &accountdomain.UserAccount{
		Id:           newID(),
		Username:     cmd.Username,
		Email:        cmd.Email,
		PasswordHash: string(hash),
		Amount:       0,
		IsVerify:     false,
	}

	if err := h.writeRepo.Save(ctx, acc); err != nil {
		return nil, err
	}

	return acc, nil
}
