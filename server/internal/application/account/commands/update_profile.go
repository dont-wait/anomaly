package commands

import (
	"context"
	"net/mail"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type UpdateCustomerProfileCommand struct {
	AccountID string
	Phone     *string
	Email     *string
	Address   *accountdomain.Address
}

type UpdateCustomerProfileHandler struct{ repo AccountRepository }

func NewUpdateCustomerProfileHandler(repo AccountRepository) *UpdateCustomerProfileHandler {
	return &UpdateCustomerProfileHandler{repo: repo}
}

func (h *UpdateCustomerProfileHandler) Handle(ctx context.Context, cmd UpdateCustomerProfileCommand) (*accountdomain.UserAccount, error) {
	account, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.Customer == nil {
		return nil, accountdomain.ErrAccountNotFound
	}
	if cmd.Phone != nil {
		phone := strings.TrimSpace(*cmd.Phone)
		if phone != "" && !validPhone(phone) {
			return nil, accountdomain.ErrInvalidProfile
		}
		account.Customer.Profile.Phone = phone
	}
	if cmd.Email != nil {
		parsed, parseErr := mail.ParseAddress(strings.TrimSpace(*cmd.Email))
		if parseErr != nil || parsed.Address != strings.TrimSpace(*cmd.Email) {
			return nil, accountdomain.ErrInvalidEmail
		}
		if existing, findErr := h.repo.FindByEmail(ctx, parsed.Address); findErr != nil {
			return nil, findErr
		} else if existing != nil && existing.Id != account.Id {
			return nil, accountdomain.ErrUserAlreadyExists
		}
		account.Email = parsed.Address
		account.Customer.Profile.Email = parsed.Address
	}
	if cmd.Address != nil {
		address := accountdomain.Address{
			Line:         strings.TrimSpace(cmd.Address.Line),
			ProvinceCode: strings.ToUpper(strings.TrimSpace(cmd.Address.ProvinceCode)),
		}
		if len(address.Line) > 200 || len(address.ProvinceCode) > 16 {
			return nil, accountdomain.ErrInvalidProfile
		}
		account.Customer.Profile.Address = address
	}
	if cmd.Phone == nil && cmd.Email == nil && cmd.Address == nil {
		return nil, accountdomain.ErrInvalidProfile
	}
	now := time.Now().UTC()
	account.Version++
	account.UpdatedAt = now
	account.Customer.UpdatedAt = now
	if err := h.repo.Save(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

func validPhone(value string) bool {
	if len(value) < 8 || len(value) > 16 {
		return false
	}
	for index, r := range value {
		if index == 0 && r == '+' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
