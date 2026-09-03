package mongo

import accountdomain "github.com/dont-wait/anomaly/internal/domain/account"

type accountRecord struct {
	Id             string `bson:"_id"`
	Username       string `bson:"username"`
	Email          string `bson:"email"`
	PasswordHash   string `bson:"passwordHash"`
	IdCardFrontUrl string `bson:"idCardFrontUrl"`
	IdCardBackUrl  string `bson:"idCardBackUrl"`
	LiveVideoUrl   string `bson:"liveVideoUrl"`
	IsVerify       bool   `bson:"isVerify"`
	Amount         int64  `bson:"amount"`
}

func toRecord(a *accountdomain.UserAccount) accountRecord {
	return accountRecord{
		Id:             a.Id,
		Username:       a.Username,
		Email:          a.Email,
		PasswordHash:   a.PasswordHash,
		IdCardFrontUrl: a.IdCardFrontUrl,
		IdCardBackUrl:  a.IdCardBackUrl,
		LiveVideoUrl:   a.LiveVideoUrl,
		IsVerify:       a.IsVerify,
		Amount:         a.Amount,
	}
}

func fromRecord(r accountRecord) *accountdomain.UserAccount {
	return &accountdomain.UserAccount{
		Id:             r.Id,
		Username:       r.Username,
		Email:          r.Email,
		PasswordHash:   r.PasswordHash,
		IdCardFrontUrl: r.IdCardFrontUrl,
		IdCardBackUrl:  r.IdCardBackUrl,
		LiveVideoUrl:   r.LiveVideoUrl,
		IsVerify:       r.IsVerify,
		Amount:         r.Amount,
	}
}
