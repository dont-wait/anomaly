package commands

import (
	"context"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// UpdateCustomerProfileCommand: sau khi đăng ký, khách chỉ tự đổi được số điện
// thoại ở đây. Email có luồng xác minh riêng, ảnh có API riêng.
type UpdateCustomerProfileCommand struct {
	AccountID string
	Phone     string
}

type UpdateCustomerProfileHandler struct{ repo AccountRepository }

func NewUpdateCustomerProfileHandler(repo AccountRepository) *UpdateCustomerProfileHandler {
	return &UpdateCustomerProfileHandler{repo: repo}
}

func (h *UpdateCustomerProfileHandler) Handle(ctx context.Context, cmd UpdateCustomerProfileCommand) (*accountdomain.UserAccount, error) {
	phone := strings.TrimSpace(cmd.Phone)
	if !validPhone(phone) {
		return nil, accountdomain.ErrInvalidProfile
	}
	account, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.Customer == nil {
		return nil, accountdomain.ErrAccountNotFound
	}
	account.Customer.Profile.Phone = phone
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
