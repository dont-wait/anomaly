package account

import (
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// AccountResponsePublic dùng cho các endpoint public (GET /api/accounts,
// /{id}, /by-email/{email}) — KHÔNG chứa identity URLs vì đây là dữ liệu
// KYC nhạy cảm (CCCD mặt trước/sau, live video), không được lộ qua API công
// khai không có authentication.
type AccountResponsePublic struct {
	Id       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	IsVerify bool   `json:"isVerify"`
	Amount   int64  `json:"amount"`
}

// AccountResponsePrivate dùng cho endpoint đã qua auth (GET /api/auth/me,
// POST /api/accounts/{id}/verify, AuthResponse). Chứa đầy đủ field kể
// cả identity URLs — chỉ user sở hữu tài khoản mới được xem KYC của mình.
type AccountResponsePrivate struct {
	Id             string `json:"id"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	IdCardFrontUrl string `json:"idCardFrontUrl"`
	IdCardBackUrl  string `json:"idCardBackUrl"`
	LiveVideoUrl   string `json:"liveVideoUrl"`
	IsVerify       bool   `json:"isVerify"`
	Amount         int64  `json:"amount"`
}

// AuthResponse trả về cho client ngay sau login — dùng private DTO vì user
// vừa xác thực và được xem KYC của chính mình.
type AuthResponse struct {
	Token     string                 `json:"token"`
	ExpiresAt time.Time              `json:"expiresAt"`
	User      AccountResponsePrivate `json:"user"`
}

func toAccountResponsePublic(a *accountdomain.UserAccount) AccountResponsePublic {
	return AccountResponsePublic{
		Id:       a.Id,
		Username: a.Username,
		Email:    a.Email,
		IsVerify: a.IsVerified(),
		Amount:   a.Balance.Current,
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
		Id:       a.Id,
		Username: a.Username,
		Email:    a.Email,
		IsVerify: a.IsVerified(),
		Amount:   a.Balance.Current,
	}
	if session := a.VerifiedKYCSession(); session != nil {
		response.IdCardFrontUrl = session.Media.IdentityFront.StorageKey
		response.IdCardBackUrl = session.Media.IdentityBack.StorageKey
		response.LiveVideoUrl = session.Media.LivenessVideo.StorageKey
	}
	return response
}

func toAuthResponse(user *accountdomain.UserAccount, token string, expiresAt time.Time) AuthResponse {
	return AuthResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      toAccountResponsePrivate(user),
	}
}
