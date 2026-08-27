package account

import accountdomain "github.com/dont-wait/anomaly/internal/domain/account"

type AccountResponse struct {
	Id       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Amount   int64  `json:"amount"`
}

func toAccountResponse(a *accountdomain.UserAccount) AccountResponse {
	return AccountResponse{
		Id:       a.Id,
		Username: a.Username,
		Email:    a.Email,
		Amount:   a.Amount,
	}
}

func toAccountResponses(list []*accountdomain.UserAccount) []AccountResponse {
	out := make([]AccountResponse, 0, len(list))
	for _, a := range list {
		out = append(out, toAccountResponse(a))
	}
	return out
}
