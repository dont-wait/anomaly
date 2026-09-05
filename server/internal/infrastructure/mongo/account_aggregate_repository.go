package mongo

import (
	"context"
	"errors"
	"fmt"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountAggregateRepository struct {
	accounts  *AccountRepository
	customers *CustomerRepository
	kyc       *KYCSessionRepository
}

func NewAccountAggregateRepository(client *mongodrv.Client, dbName string) *AccountAggregateRepository {
	return &AccountAggregateRepository{
		accounts:  NewAccountRepository(client, dbName),
		customers: NewCustomerRepository(client, dbName),
		kyc:       NewKYCSessionRepository(client, dbName),
	}
}

func (r *AccountAggregateRepository) EnsureIndexes(ctx context.Context) error {
	if err := r.accounts.EnsureIndexes(ctx); err != nil {
		return err
	}
	if err := r.customers.EnsureIndexes(ctx); err != nil {
		return err
	}
	return r.kyc.EnsureIndexes(ctx)
}

func (r *AccountAggregateRepository) Create(ctx context.Context, account *accountdomain.UserAccount) error {
	if account.Customer == nil {
		return fmt.Errorf("create account %s: customer is missing", account.Id)
	}
	if err := r.accounts.Save(ctx, account); err != nil {
		if IsDuplicateKeyError(err) {
			return accountdomain.ErrUserAlreadyExists
		}
		return err
	}
	if err := r.customers.Save(ctx, account.Customer); err != nil {
		if rollbackErr := r.accounts.DeleteByID(ctx, account.Id); rollbackErr != nil {
			return fmt.Errorf("create customer failed and account rollback failed: %w", errors.Join(err, rollbackErr))
		}
		return err
	}
	return nil
}

func (r *AccountAggregateRepository) Save(ctx context.Context, account *accountdomain.UserAccount) error {
	if account.Customer == nil {
		return fmt.Errorf("save account %s: customer is missing", account.Id)
	}
	for _, session := range account.KYCSessions {
		if err := r.kyc.Save(ctx, session); err != nil {
			return err
		}
	}
	if err := r.customers.Save(ctx, account.Customer); err != nil {
		return err
	}
	if err := r.accounts.Save(ctx, account); err != nil {
		if IsDuplicateKeyError(err) {
			return accountdomain.ErrUserAlreadyExists
		}
		return err
	}
	return nil
}

func (r *AccountAggregateRepository) FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error) {
	account, err := r.accounts.FindByID(ctx, id)
	return r.hydrate(ctx, account, err)
}

func (r *AccountAggregateRepository) FindByEmail(ctx context.Context, email string) (*accountdomain.UserAccount, error) {
	account, err := r.accounts.FindByEmail(ctx, email)
	return r.hydrate(ctx, account, err)
}

func (r *AccountAggregateRepository) FindByUsername(ctx context.Context, username string) (*accountdomain.UserAccount, error) {
	account, err := r.accounts.FindByUsername(ctx, username)
	return r.hydrate(ctx, account, err)
}

func (r *AccountAggregateRepository) FindAll(ctx context.Context) ([]*accountdomain.UserAccount, error) {
	accounts, err := r.accounts.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for index, account := range accounts {
		accounts[index], err = r.hydrate(ctx, account, nil)
		if err != nil {
			return nil, err
		}
	}
	return accounts, nil
}

func (r *AccountAggregateRepository) hydrate(
	ctx context.Context,
	account *accountdomain.UserAccount,
	err error,
) (*accountdomain.UserAccount, error) {
	if err != nil || account == nil {
		return account, err
	}
	account.Customer, err = r.customers.FindByID(ctx, account.CustomerId)
	if err != nil {
		return nil, err
	}
	account.KYCSessions, err = r.kyc.FindByCustomerID(ctx, account.CustomerId)
	if err != nil {
		return nil, err
	}
	return account, nil
}
