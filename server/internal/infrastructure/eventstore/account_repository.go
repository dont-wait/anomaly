package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// AccountRepository implement cùng port với mongo.AccountRepository
// (commands.AccountRepository + queries.AccountQueryRepository), nhưng
// dùng event store thay vì MongoDB. Chạy song song với Mongo repository
// (mục đích học/thử nghiệm), không thay thế.
type AccountRepository struct {
	client *kurrentdb.Client
}

func NewAccountRepository(client *kurrentdb.Client) *AccountRepository {
	return &AccountRepository{client: client}
}

// Save ghi thêm event cần thiết để đưa stream từ trạng thái hiện tại
// tới trạng thái của `a`:
//   - Stream chưa tồn tại -> ghi AccountCreated
//   - Stream đã tồn tại -> ghi AccountWithdrew với phần chênh lệch
//     (chỉ hỗ trợ rút tiền, đúng với logic domain hiện tại)
func (r *AccountRepository) Save(ctx context.Context, a *accountdomain.UserAccount) error {
	current, err := r.FindByID(ctx, a.Id)
	if err != nil {
		return err
	}

	stream := streamName(a.Id)

	if current == nil {
		payload, err := json.Marshal(accountCreatedPayload{
			Id:       a.Id,
			Username: a.Username,
			Email:    a.Email,
		})
		if err != nil {
			return err
		}

		_, err = r.client.AppendToStream(ctx, stream, kurrentdb.AppendToStreamOptions{
			StreamState: kurrentdb.NoStream{},
		}, kurrentdb.EventData{
			ContentType: kurrentdb.ContentTypeJson,
			EventType:   eventAccountCreated,
			Data:        payload,
		})
		return err
	}

	delta := current.Amount - a.Amount
	if delta <= 0 {
		return nil
	}

	payload, err := json.Marshal(accountWithdrewPayload{Amount: delta})
	if err != nil {
		return err
	}

	_, err = r.client.AppendToStream(ctx, stream, kurrentdb.AppendToStreamOptions{}, kurrentdb.EventData{
		ContentType: kurrentdb.ContentTypeJson,
		EventType:   eventAccountWithdrew,
		Data:        payload,
	})
	return err
}

// FindByID dựng lại trạng thái hiện tại bằng cách đọc và replay
// toàn bộ event trong stream, theo đúng thứ tự.
func (r *AccountRepository) FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error) {
	stream, err := r.client.ReadStream(ctx, streamName(id), kurrentdb.ReadStreamOptions{}, 4096)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	acc := &accountdomain.UserAccount{}
	found := false

	for {
		event, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if kdbErr, ok := kurrentdb.FromError(err); ok && kdbErr.Code() == kurrentdb.ErrorCodeResourceNotFound {
				return nil, nil
			}
			return nil, err
		}

		found = true

		decode := func(v any) error {
			return json.Unmarshal(event.Event.Data, v)
		}
		if err := applyEvent(acc, event.Event.EventType, decode); err != nil {
			return nil, err
		}
	}

	if !found {
		return nil, nil
	}
	return acc, nil
}

// FindByEmail và FindAll phải quét qua mọi stream account, vì event
// store không có sẵn index theo email (việc đó thường do projection
// đảm nhận). Đây là bản đơn giản, chưa tối ưu, dùng cho mục đích
// học/song song với Mongo - production thật nên xây read-model riêng
// (projection của KurrentDB, hoặc consumer Kafka cập nhật 1 store đọc).

func (r *AccountRepository) FindByEmail(ctx context.Context, email string) (*accountdomain.UserAccount, error) {
	accounts, err := r.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		if acc.Email == email {
			return acc, nil
		}
	}
	return nil, nil
}

func (r *AccountRepository) FindAll(ctx context.Context) ([]*accountdomain.UserAccount, error) {
	ids, err := r.allAccountIDs(ctx)
	if err != nil {
		return nil, err
	}

	accounts := make([]*accountdomain.UserAccount, 0, len(ids))
	for _, id := range ids {
		acc, err := r.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if acc != nil {
			accounts = append(accounts, acc)
		}
	}
	return accounts, nil
}

// allAccountIDs quét $all để lấy danh sách account ID từ các event
// AccountCreated.
func (r *AccountRepository) allAccountIDs(ctx context.Context) ([]string, error) {
	all, err := r.client.ReadAll(ctx, kurrentdb.ReadAllOptions{}, 4096)
	if err != nil {
		return nil, err
	}
	defer all.Close()

	var ids []string
	for {
		event, err := all.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		if event.Event.EventType != eventAccountCreated {
			continue
		}

		var p accountCreatedPayload
		if err := json.Unmarshal(event.Event.Data, &p); err != nil {
			return nil, err
		}
		ids = append(ids, p.Id)
	}
	return ids, nil
}
