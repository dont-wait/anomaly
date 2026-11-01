package account

import (
	"path"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// AccountResponsePublic dùng cho các endpoint tra cứu account và không chứa
// identity URLs vì đây là dữ liệu KYC nhạy cảm.
type AccountResponsePublic struct {
	Id        string `json:"id"`
	AccountNo string `json:"accountNo"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Currency  string `json:"currency"`
	Role      string `json:"role"`
	IsVerify  bool   `json:"isVerify"`
	Amount    int64  `json:"amount"`
}

// AccountResponsePrivate dùng cho endpoint đã qua auth (GET /api/auth/me,
// POST /api/kyc/complete, AuthResponse). Chứa đầy đủ field kể
// cả identity URLs — chỉ user sở hữu tài khoản mới được xem KYC của mình.
type AccountResponsePrivate struct {
	Id            string                  `json:"id"`
	AccountNo     string                  `json:"accountNo"`
	Username      string                  `json:"username"`
	FullName      string                  `json:"fullName"`
	Email         string                  `json:"email"`
	Currency      string                  `json:"currency"`
	Role          string                  `json:"role"`
	IsVerify      bool                    `json:"isVerify"`
	Amount        int64                   `json:"amount"`
	Phone         string                  `json:"phone,omitempty"`
	DateOfBirth   *time.Time              `json:"dateOfBirth,omitempty"`
	Address       ProfileAddressResponse  `json:"address,omitempty"`
	Identity      ProfileIdentityResponse `json:"identity,omitempty"`
	Avatar        *ProfileMediaResponse   `json:"avatar,omitempty"`
	ProfileChange *ProfileChangeResponse  `json:"profileChange,omitempty"`
	PendingEmail  *string                 `json:"pendingEmail"`
}

type ProfileAddressResponse struct {
	Line         string `json:"line,omitempty"`
	ProvinceCode string `json:"provinceCode,omitempty"`
}

type ProfileIdentityResponse struct {
	Type             string     `json:"type,omitempty"`
	Number           string     `json:"number,omitempty"`
	IssuedDate       *time.Time `json:"issuedDate,omitempty"`
	IssuedPlace      string     `json:"issuedPlace,omitempty"`
	PermanentAddress string     `json:"permanentAddress,omitempty"`
}

type ProfileMediaResponse struct {
	MediaID     string    `json:"mediaId"`
	Type        string    `json:"type"`
	FileName    string    `json:"fileName"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	UploadedAt  time.Time `json:"uploadedAt"`
	Status      string    `json:"status"`
}

// ProfileChangeResponse hiện đề nghị đổi thông tin; trường rỗng là không đổi.
type ProfileChangeResponse struct {
	ID               string     `json:"id"`
	FullName         string     `json:"fullName,omitempty"`
	AddressLine      string     `json:"addressLine,omitempty"`
	ProvinceCode     string     `json:"provinceCode,omitempty"`
	IssuedPlace      string     `json:"issuedPlace,omitempty"`
	PermanentAddress string     `json:"permanentAddress,omitempty"`
	Status           string     `json:"status"`
	RejectionReason  string     `json:"rejectionReason,omitempty"`
	RequestedAt      time.Time  `json:"requestedAt"`
	ReviewedAt       *time.Time `json:"reviewedAt,omitempty"`
}

// ProfileChangeReviewItem là một đề nghị kèm thông tin hiện tại để admin so sánh.
type ProfileChangeReviewItem struct {
	AccountID string                 `json:"accountId"`
	Username  string                 `json:"username"`
	Current   ProfileChangeCurrent   `json:"current"`
	Change    *ProfileChangeResponse `json:"change"`
}

type ProfileChangeCurrent struct {
	FullName         string `json:"fullName"`
	AddressLine      string `json:"addressLine"`
	ProvinceCode     string `json:"provinceCode"`
	IssuedPlace      string `json:"issuedPlace"`
	PermanentAddress string `json:"permanentAddress"`
}

func toProfileChangeResponse(c *accountdomain.ProfileChangeRequest) *ProfileChangeResponse {
	if c == nil {
		return nil
	}
	return &ProfileChangeResponse{ID: c.Id, FullName: c.FullName, AddressLine: c.AddressLine, ProvinceCode: c.ProvinceCode, IssuedPlace: c.IssuedPlace, PermanentAddress: c.PermanentAddress, Status: c.Status, RejectionReason: c.RejectionReason, RequestedAt: c.RequestedAt, ReviewedAt: c.ReviewedAt}
}

// ProfileDocumentSlot is one document type the backend supports for this account.
type ProfileDocumentSlot struct {
	Type        string                `json:"type"`
	Required    bool                  `json:"required"`
	Replaceable bool                  `json:"replaceable"`
	Document    *ProfileMediaResponse `json:"document"`
}

type ProfileDocumentsResponse struct {
	Slots []ProfileDocumentSlot `json:"slots"`
}

// AuthResponse trả về cho client ngay sau login — dùng private DTO vì user
// vừa xác thực và được xem KYC của chính mình.
type AuthResponse struct {
	Token     string                 `json:"token"`
	ExpiresAt time.Time              `json:"expiresAt"`
	User      AccountResponsePrivate `json:"user"`
}

type RegisterResponse struct {
	User         AccountResponsePrivate `json:"user"`
	KYCToken     string                 `json:"kycToken,omitempty"`
	KYCExpiresAt *time.Time             `json:"kycExpiresAt,omitempty"`
}

type KYCSessionResponse = RegisterResponse

type KYCCompleteResponse struct {
	Decision          string                  `json:"decision"`
	ReasonCode        string                  `json:"reasonCode,omitempty"`
	ReasonMessage     string                  `json:"reasonMessage,omitempty"`
	User              *AccountResponsePrivate `json:"user,omitempty"`
	AttemptsRemaining *int                    `json:"attemptsRemaining,omitempty"`
	AttemptLimit      int                     `json:"attemptLimit,omitempty"`
	RetryAfter        *time.Time              `json:"retryAfter,omitempty"`
}

func toAccountResponsePublic(a *accountdomain.UserAccount) AccountResponsePublic {
	return AccountResponsePublic{
		Id:        a.Id,
		AccountNo: a.AccountNo,
		Username:  a.Username,
		Email:     a.Email,
		Currency:  string(a.Currency),
		Role:      string(a.EffectiveRole()),
		IsVerify:  a.IsVerified(),
		Amount:    a.Balance.Current,
	}
}

func toAccountResponsePublicList(list []*accountdomain.UserAccount) []AccountResponsePublic {
	out := make([]AccountResponsePublic, 0, len(list))
	for _, a := range list {
		out = append(out, toAccountResponsePublic(a))
	}
	return out
}

func toAccountResponsePrivate(a *accountdomain.UserAccount) AccountResponsePrivate {
	response := AccountResponsePrivate{
		Id:        a.Id,
		AccountNo: a.AccountNo,
		Username:  a.Username,
		FullName:  a.Username,
		Email:     a.Email,
		Currency:  string(a.Currency),
		Role:      string(a.EffectiveRole()),
		IsVerify:  a.IsVerified(),
		Amount:    a.Balance.Current,
	}
	if a.Customer != nil && a.Customer.Profile.FullName != "" {
		response.FullName = a.Customer.Profile.FullName
	}
	return response
}

func toProfileResponse(a *accountdomain.UserAccount) AccountResponsePrivate {
	response := toAccountResponsePrivate(a)
	if a.Customer == nil {
		return response
	}
	profile := a.Customer.Profile
	response.Phone = profile.Phone
	response.DateOfBirth = profile.DateOfBirth
	response.Address = ProfileAddressResponse{Line: profile.Address.Line, ProvinceCode: profile.Address.ProvinceCode}
	identity := a.Customer.Identity
	response.Identity = ProfileIdentityResponse{Type: identity.Type, Number: maskIdentity(identity.Number), IssuedDate: identity.IssuedDate, IssuedPlace: identity.IssuedPlace, PermanentAddress: identity.PermanentAddress}
	response.ProfileChange = toProfileChangeResponse(a.Customer.ProfileChange)
	if a.Customer.Avatar != nil && a.Customer.Avatar.Status == "ACTIVE" {
		response.Avatar = toProfileMediaResponse(*a.Customer.Avatar)
	}
	return response
}

// toProfileDocumentSlots applies the first-release document policy: verified
// identity evidence is read-only, other documents are optional in the profile.
func toProfileDocumentSlots(a *accountdomain.UserAccount) []ProfileDocumentSlot {
	slots := []ProfileDocumentSlot{
		{Type: "IDENTITY_FRONT", Required: true},
		{Type: "IDENTITY_BACK", Required: true},
		{Type: "INCOME_PROOF", Replaceable: true},
		{Type: "ADDRESS_PROOF", Replaceable: true},
		// Chứng từ vay: tải một lần ở profile, mỗi hồ sơ vay giữ bản chụp riêng.
		{Type: "VNEID_PERSONAL", Replaceable: true},
		{Type: "VNEID_RESIDENCE", Replaceable: true},
		{Type: "VSSID_PARTICIPATION", Replaceable: true},
		{Type: "VSSID_DETAIL", Replaceable: true},
		{Type: "SALARY_STATEMENT_3_MONTHS", Replaceable: true},
	}
	if a.Customer != nil {
		for index := range slots {
			for _, document := range a.Customer.Documents {
				if document.Type == slots[index].Type && document.Status == "ACTIVE" {
					slots[index].Document = toProfileMediaResponse(document)
				}
			}
		}
	}
	if session := a.VerifiedKYCSession(); session != nil {
		slots[0].Document = kycMediaResponse("kyc-identity-front", "IDENTITY_FRONT", session.Media.IdentityFront, session.CreatedAt)
		slots[1].Document = kycMediaResponse("kyc-identity-back", "IDENTITY_BACK", session.Media.IdentityBack, session.CreatedAt)
	}
	return slots
}

func maskIdentity(value string) string {
	if len(value) <= 4 {
		return value
	}
	return strings.Repeat("•", len(value)-4) + value[len(value)-4:]
}

func toProfileMediaResponse(media accountdomain.ProfileMedia) *ProfileMediaResponse {
	return &ProfileMediaResponse{MediaID: media.ID, Type: media.Type, FileName: media.FileName, ContentType: media.ContentType, SizeBytes: media.SizeBytes, UploadedAt: media.UploadedAt, Status: media.Status}
}

func kycMediaResponse(id, kind string, media accountdomain.MediaObject, uploadedAt time.Time) *ProfileMediaResponse {
	return &ProfileMediaResponse{MediaID: id, Type: kind, FileName: path.Base(media.StorageKey), ContentType: media.MIMEType, SizeBytes: media.SizeBytes, UploadedAt: uploadedAt, Status: "VERIFIED"}
}

func toAuthResponse(user *accountdomain.UserAccount, token string, expiresAt time.Time) AuthResponse {
	return AuthResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      toProfileResponse(user),
	}
}
