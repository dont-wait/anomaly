package commands

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// ProfileChangeInput: trường rỗng là không đổi. Số CCCD, ngày sinh và ngày cấp
// không nằm ở đây nên không thể đổi qua luồng này.
type ProfileChangeInput struct {
	FullName         string
	AddressLine      string
	ProvinceCode     string
	IssuedPlace      string
	PermanentAddress string
}

// ProfileChangeHandler lưu đề nghị của khách. Hồ sơ thật chỉ đổi khi admin duyệt.
type ProfileChangeHandler struct{ repo AccountRepository }

func NewProfileChangeHandler(repo AccountRepository) *ProfileChangeHandler {
	return &ProfileChangeHandler{repo: repo}
}

// Submit thay thế đề nghị đang chờ, nếu có, bằng đề nghị mới.
func (h *ProfileChangeHandler) Submit(ctx context.Context, accountID string, input ProfileChangeInput) (*accountdomain.UserAccount, error) {
	request := &accountdomain.ProfileChangeRequest{
		Id:               newID(),
		FullName:         strings.TrimSpace(input.FullName),
		AddressLine:      strings.TrimSpace(input.AddressLine),
		ProvinceCode:     strings.ToUpper(strings.TrimSpace(input.ProvinceCode)),
		IssuedPlace:      strings.TrimSpace(input.IssuedPlace),
		PermanentAddress: strings.TrimSpace(input.PermanentAddress),
		Status:           accountdomain.ProfileChangePending,
		RequestedAt:      time.Now().UTC(),
	}
	if request.FullName == "" && request.AddressLine == "" && request.ProvinceCode == "" && request.IssuedPlace == "" && request.PermanentAddress == "" {
		return nil, accountdomain.ErrInvalidProfile
	}
	if utf8.RuneCountInString(request.FullName) > 120 || utf8.RuneCountInString(request.AddressLine) > 200 ||
		len(request.ProvinceCode) > 16 || utf8.RuneCountInString(request.IssuedPlace) > 200 ||
		utf8.RuneCountInString(request.PermanentAddress) > 200 {
		return nil, accountdomain.ErrInvalidProfile
	}
	account, err := h.repo.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.Customer == nil {
		return nil, accountdomain.ErrAccountNotFound
	}
	account.Customer.ProfileChange = request
	touchAccount(account)
	if err := h.repo.Save(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// Decide duyệt hoặc từ chối đúng đề nghị admin đang xem.
func (h *ProfileChangeHandler) Decide(ctx context.Context, adminID, accountID, requestID, decision, reason string) error {
	if adminID == accountID {
		return accountdomain.ErrSelfProfileReview
	}
	reason = strings.TrimSpace(reason)
	switch decision {
	case accountdomain.ProfileChangeApproved:
		reason = ""
	case accountdomain.ProfileChangeRejected:
		if reason == "" || utf8.RuneCountInString(reason) > 500 {
			return accountdomain.ErrInvalidProfile
		}
	default:
		return accountdomain.ErrInvalidProfile
	}
	account, err := h.repo.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account == nil || account.Customer == nil {
		return accountdomain.ErrAccountNotFound
	}
	request := account.Customer.ProfileChange
	if request == nil || request.Id != requestID || request.Status != accountdomain.ProfileChangePending {
		return accountdomain.ErrProfileChangeClosed
	}
	if decision == accountdomain.ProfileChangeApproved {
		applyProfileChange(account.Customer, request)
	}
	now := time.Now().UTC()
	request.Status = decision
	request.RejectionReason = reason
	request.ReviewedAt = &now
	request.ReviewedBy = adminID
	touchAccount(account)
	return h.repo.Save(ctx, account)
}

func applyProfileChange(customer *accountdomain.Customer, request *accountdomain.ProfileChangeRequest) {
	for _, field := range []struct {
		value  string
		target *string
	}{
		{request.FullName, &customer.Profile.FullName},
		{request.AddressLine, &customer.Profile.Address.Line},
		{request.ProvinceCode, &customer.Profile.Address.ProvinceCode},
		{request.IssuedPlace, &customer.Identity.IssuedPlace},
		{request.PermanentAddress, &customer.Identity.PermanentAddress},
	} {
		if field.value != "" {
			*field.target = field.value
		}
	}
}
