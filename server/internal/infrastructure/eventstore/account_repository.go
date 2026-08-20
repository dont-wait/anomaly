package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	current, currentRevision, err := r.findByIDWithRevision(ctx, a.Id)
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

	_, err = r.client.AppendToStream(ctx, stream, kurrentdb.AppendToStreamOptions{
		StreamState: kurrentdb.StreamRevision{Value: currentRevision},
	}, kurrentdb.EventData{
		ContentType: kurrentdb.ContentTypeJson,
		EventType:   eventAccountWithdrew,
		Data:        payload,
	})
	if err != nil {
		if kdbErr, ok := kurrentdb.FromError(err); ok && kdbErr.Code() == kurrentdb.ErrorCodeWrongExpectedVersion {
			return fmt.Errorf("account %s was modified concurrently, please retry: %w", a.Id, err)
		}
		return err
	}
	return err
}

// FindByID dựng lại trạng thái hiện tại bằng cách đọc và replay TOÀN BỘ event trong stream.
func (r *AccountRepository) FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error) {
	acc, _, err := r.findByIDWithRevision(ctx, id)
	return acc, err
}

// findByIDWithRevision làm y hệt FindByID, nhưng trả thêm revision của event CUỐI CÙNG đã đọc được - dùng để kiểm tra tranh chấp (optimistic
// concurrency) khi Save() ghi tiếp vào cùng stream này.
func (r *AccountRepository) findByIDWithRevision(ctx context.Context, id string) (*accountdomain.UserAccount, uint64, error) {
	const pageSize = 4096

	acc := &accountdomain.UserAccount{}
	found := false
	var lastRevision uint64

	opts := kurrentdb.ReadStreamOptions{}

	for {
		stream, err := r.client.ReadStream(ctx, streamName(id), opts, pageSize)
		if err != nil {
			return nil, 0, err
		}

		eventsInPage := 0

		for {
			event, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				stream.Close()
				if kdbErr, ok := kurrentdb.FromError(err); ok && kdbErr.Code() == kurrentdb.ErrorCodeResourceNotFound {
					return nil, 0, nil
				}
				return nil, 0, err
			}

			found = true
			eventsInPage++
			lastRevision = event.Event.EventNumber

			decode := func(v any) error {
				return json.Unmarshal(event.Event.Data, v)
			}
			if err := applyEvent(acc, event.Event.EventType, decode); err != nil {
				stream.Close()
				return nil, 0, err
			}
		}
		stream.Close()

		if eventsInPage < pageSize {
			break
		}

		opts = kurrentdb.ReadStreamOptions{From: kurrentdb.Revision(lastRevision + 1)}
	}

	if !found {
		return nil, 0, nil
	}
	return acc, lastRevision, nil
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
	const pageSize = 4096

	seen := make(map[string]struct{})
	var ids []string
	opts := kurrentdb.ReadAllOptions{} // trang đầu tiên: đọc từ Start

	for {
		all, err := r.client.ReadAll(ctx, opts, pageSize)
		if err != nil {
			return nil, err
		}

		eventsInPage := 0
		var lastPosition kurrentdb.Position

		for {
			event, err := all.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				all.Close()
				return nil, err
			}

			eventsInPage++
			lastPosition = event.Event.Position // vị trí trong $all (khác revision trong 1 stream)

			if event.Event.EventType == eventAccountCreated {
				var p accountCreatedPayload
				if err := json.Unmarshal(event.Event.Data, &p); err != nil {
					all.Close()
					return nil, err
				}
				// ReadAll với From: Position là inclusive, nên event tại
				// lastPosition của trang trước sẽ được trả lại lần nữa ở
				// đầu trang sau -> dedupe theo id để tránh account bị lặp.
				if _, dup := seen[p.Id]; !dup {
					seen[p.Id] = struct{}{}
					ids = append(ids, p.Id)
				}
			}
		}
		all.Close()

		if eventsInPage < pageSize {
			break
		}

		// From là inclusive nên trang kế tiếp đọc lại đúng event tại
		// lastPosition; dedupe ở trên xử lý phần trùng đó.
		opts = kurrentdb.ReadAllOptions{From: lastPosition}
	}

	return ids, nil
}
