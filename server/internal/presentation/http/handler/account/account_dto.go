package account

import (
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountResponse struct {
	Id             string `json:"id"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	IdCardFrontUrl string `json:"idCardFrontUrl"`
	IdCardBackUrl  string `json:"idCardBackUrl"`
	LiveVideoUrl   string `json:"liveVideoUrl"`
	IsVerify       bool   `json:"isVerify"`
	Amount         int64  `json:"amount"`
}

type AuthResponse struct {
	Token     string          `json:"token"`
	ExpiresAt time.Time       `json:"expiresAt"`
	User      AccountResponse `json:"user"`
}

func toAccountResponse(a *accountdomain.UserAccount) AccountResponse {
	return AccountResponse{
		Id:             a.Id,
		Username:       a.Username,
		Email:          a.Email,
		IdCardFrontUrl: a.IdCardFrontUrl,
		IdCardBackUrl:  a.IdCardBackUrl,
		LiveVideoUrl:   a.LiveVideoUrl,
		IsVerify:       a.IsVerify,
		Amount:         a.Amount,
	}
}

func toAccountResponses(list []*accountdomain.UserAccount) []AccountResponse {
	out := make([]AccountResponse, 0, len(list))
	for _, a := range list {
		out = append(out, toAccountResponse(a))
	}
	return out
}

func toAuthResponse(user *accountdomain.UserAccount, token string, expiresAt time.Time) AuthResponse {
	return AuthResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      toAccountResponse(user),
	}
}
