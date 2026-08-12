package account

type UserAccount struct {
	Id       string
	Username string
	Email    string
	Amount   int64
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
