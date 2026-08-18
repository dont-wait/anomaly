package mongo

import accountdomain "github.com/dont-wait/anomaly/internal/domain/account"

type accountRecord struct {
	Id       string `bson:"_id"`
	Username string `bson:"username"`
	Email    string `bson:"email"`
	Amount   int64  `bson:"amount"`
}

func toRecord(a *accountdomain.UserAccount) accountRecord {
	return accountRecord{
		Id:       a.Id,
		Username: a.Username,
		Email:    a.Email,
		Amount:   a.Amount,
	}
}

func fromRecord(r accountRecord) *accountdomain.UserAccount {
	return &accountdomain.UserAccount{
		Id:       r.Id,
		Username: r.Username,
		Email:    r.Email,
		Amount:   r.Amount,
	}
}
