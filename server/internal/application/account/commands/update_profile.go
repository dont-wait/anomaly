package commands

import (
	"context"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// UpdateCustomerProfileCommand is a partial update: a nil field keeps its current value.
type UpdateCustomerProfileCommand struct {
	AccountID    string
	Phone        *string
	AddressLine  *string
	ProvinceCode *string
}

type UpdateCustomerProfileHandler struct{ repo AccountRepository }

func NewUpdateCustomerProfileHandler(repo AccountRepository) *UpdateCustomerProfileHandler {
	return &UpdateCustomerProfileHandler{repo: repo}
}

func (h *UpdateCustomerProfileHandler) Handle(ctx context.Context, cmd UpdateCustomerProfileCommand) (*accountdomain.UserAccount, error) {
	if cmd.Phone == nil && cmd.AddressLine == nil && cmd.ProvinceCode == nil {
		return nil, accountdomain.ErrInvalidProfile
	}
	account, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.Customer == nil {
		return nil, accountdomain.ErrAccountNotFound
	}
	if cmd.Phone != nil {
		phone := strings.TrimSpace(*cmd.Phone)
		if !validPhone(phone) {
			return nil, accountdomain.ErrInvalidProfile
		}
		account.Customer.Profile.Phone = phone
	}
	if cmd.AddressLine != nil {
		line := strings.TrimSpace(*cmd.AddressLine)
		if line == "" || len(line) > 200 {
			return nil, accountdomain.ErrInvalidProfile
		}
		account.Customer.Profile.Address.Line = line
	}
	if cmd.ProvinceCode != nil {
		province := strings.ToUpper(strings.TrimSpace(*cmd.ProvinceCode))
		if province == "" || len(province) > 16 {
			return nil, accountdomain.ErrInvalidProfile
		}
		account.Customer.Profile.Address.ProvinceCode = province
	}
	touchAccount(account)
	if err := h.repo.Save(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// touchAccount must precede Save: BankRepository.Save writes nothing when Version is unchanged.
func touchAccount(account *accountdomain.UserAccount) {
	now := time.Now().UTC()
	account.Version++
	account.UpdatedAt = now
	if account.Customer != nil {
		account.Customer.UpdatedAt = now
	}
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
