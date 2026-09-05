package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountAggregateRepository struct {
	accounts  *AccountRepository
	customers *CustomerRepository
	kyc       *KYCSessionRepository
}

const compensationTimeout = 5 * time.Second

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
	if err := validateAccountAggregate(account); err != nil {
		return err
	}
	if err := r.accounts.Save(ctx, account); err != nil {
		if IsDuplicateKeyError(err) {
			return accountdomain.ErrUserAlreadyExists
		}
		return err
	}
	if err := r.customers.Save(ctx, account.Customer); err != nil {
		return r.compensateCreate(ctx, account, err)
	}
	for _, session := range account.KYCSessions {
		if err := r.kyc.Save(ctx, session); err != nil {
			return r.compensateCreate(ctx, account, err)
		}
	}
	return nil
}

func (r *AccountAggregateRepository) Save(ctx context.Context, account *accountdomain.UserAccount) error {
	if err := validateAccountAggregate(account); err != nil {
		return err
	}
	previous, err := r.FindByID(ctx, account.Id)
	if err != nil {
		return err
	}
	if previous == nil {
		return r.Create(ctx, account)
	}
	for _, session := range account.KYCSessions {
		if err := r.kyc.Save(ctx, session); err != nil {
			return r.compensateUpdate(ctx, previous, err)
		}
	}
	if err := r.customers.Save(ctx, account.Customer); err != nil {
		return r.compensateUpdate(ctx, previous, err)
	}
	if err := r.accounts.Save(ctx, account); err != nil {
		if IsDuplicateKeyError(err) {
			err = accountdomain.ErrUserAlreadyExists
		}
		return r.compensateUpdate(ctx, previous, err)
	}
	return nil
}

func validateAccountAggregate(account *accountdomain.UserAccount) error {
	if account.Customer == nil {
		return fmt.Errorf("account %s: customer is missing", account.Id)
	}
	if _, err := toRecord(account); err != nil {
		return err
	}
	if _, err := toCustomerRecord(account.Customer); err != nil {
		return err
	}
	for _, session := range account.KYCSessions {
		if _, err := toKYCSessionRecord(session); err != nil {
			return err
		}
	}
	return nil
}

func (r *AccountAggregateRepository) compensateCreate(
	ctx context.Context,
	account *accountdomain.UserAccount,
	cause error,
) error {
	compensationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), compensationTimeout)
	defer cancel()

	return joinCompensationErrors(cause,
		wrapCompensationError("delete KYC sessions", r.kyc.DeleteByCustomerID(compensationCtx, account.CustomerId)),
		wrapCompensationError("delete customer", r.customers.DeleteByID(compensationCtx, account.CustomerId)),
		wrapCompensationError("delete account", r.accounts.DeleteByID(compensationCtx, account.Id)),
	)
}

func (r *AccountAggregateRepository) compensateUpdate(
	ctx context.Context,
	previous *accountdomain.UserAccount,
	cause error,
) error {
	compensationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), compensationTimeout)
	defer cancel()

	compensationErrors := []error{
		wrapCompensationError("restore account", r.accounts.Save(compensationCtx, previous)),
	}
	if previous.Customer == nil {
		compensationErrors = append(compensationErrors,
			wrapCompensationError("delete customer", r.customers.DeleteByID(compensationCtx, previous.CustomerId)),
		)
	} else {
		compensationErrors = append(compensationErrors,
			wrapCompensationError("restore customer", r.customers.Save(compensationCtx, previous.Customer)),
		)
	}
	compensationErrors = append(compensationErrors,
		wrapCompensationError("delete current KYC sessions", r.kyc.DeleteByCustomerID(compensationCtx, previous.CustomerId)),
	)
	for _, session := range previous.KYCSessions {
		compensationErrors = append(compensationErrors,
			wrapCompensationError("restore KYC session", r.kyc.Save(compensationCtx, session)),
		)
	}
	return joinCompensationErrors(cause, compensationErrors...)
}

func wrapCompensationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("compensation %s failed: %w", operation, err)
}

func joinCompensationErrors(cause error, compensationErrors ...error) error {
	joined := errors.Join(compensationErrors...)
	if joined == nil {
		return cause
	}
	return errors.Join(cause, joined)
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
