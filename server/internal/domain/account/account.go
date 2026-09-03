package account

type UserAccount struct {
	Id             string
	Username       string
	Email          string
	PasswordHash   string
	IdCardFrontUrl string
	IdCardBackUrl  string
	LiveVideoUrl   string
	IsVerify       bool
	Amount         int64
}

func (u *UserAccount) Withdraw(amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if amount > u.Amount {
		return ErrInsufficientFunds
	}

	u.Amount -= amount
	return nil
}
