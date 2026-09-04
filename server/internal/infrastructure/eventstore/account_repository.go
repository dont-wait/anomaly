package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	eventstoredb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const PAGESIZE = 4096

// AccountRepository implement cùng port với mongo.AccountRepository
// (commands.AccountRepository + queries.AccountQueryRepository), nhưng
// dùng event store thay vì MongoDB. Chạy song song với Mongo repository
// (mục đích học/thử nghiệm), không thay thế.
type AccountRepository struct {
	client *eventstoredb.Client
}

func NewAccountRepository(client *eventstoredb.Client) *AccountRepository {
	return &AccountRepository{client: client}
}

// Save ghi event phù hợp để đưa stream từ trạng thái hiện tại
// tới trạng thái của `a`:
//   - Stream chưa tồn tại -> ghi AccountCreated (lần đầu register)
//   - Stream đã tồn tại + identity URLs hoặc IsVerify thay đổi -> ghi AccountVerified
//   - Stream đã tồn tại + Amount giảm -> ghi AccountWithdraw
//
// Lưu ý: AccountVerified được emit mỗi lần identity thay đổi, không chỉ ở
// lần verify đầu tiên — đảm bảo audit log đầy đủ cho re-KYC.
func (r *AccountRepository) Save(ctx context.Context, a *accountdomain.UserAccount) error {
	current, currentRevision, err := r.findByIDWithRevision(ctx, a.Id)
	if err != nil {
		return err
	}

	stream := streamName(a.Id)

	if current == nil {
		payload, err := json.Marshal(accountCreatedPayload{
			Account: a,
		})
		if err != nil {
			return err
		}

		_, err = r.client.AppendToStream(
			ctx,
			stream,
			eventstoredb.AppendToStreamOptions{
				StreamState: eventstoredb.NoStream{},
			}, eventstoredb.EventData{
				ContentType: eventstoredb.ContentTypeJson,
				EventType:   EventAccountCreated,
				Data:        payload,
			})
		return err
	}

	if kycSessionAdded(current, a) {
		session := a.KYCSessions[len(a.KYCSessions)-1]
		payload, err := json.Marshal(accountVerifiedPayload{
			Session:              session,
			VerifiedKYCSessionId: a.Customer.VerifiedKYCSessionId,
			Version:              &a.Version,
			UpdatedAt:            &a.UpdatedAt,
		})
		if err != nil {
			return err
		}

		_, err = r.client.AppendToStream(
			ctx,
			stream,
			eventstoredb.AppendToStreamOptions{
				StreamState: eventstoredb.StreamRevision{Value: currentRevision},
			}, eventstoredb.EventData{
				ContentType: eventstoredb.ContentTypeJson,
				EventType:   EventAccountVerified,
				Data:        payload,
			})
		if err != nil {
			var eventStoreErr *eventstoredb.Error
			if errors.As(err, &eventStoreErr) && eventStoreErr.Code() == eventstoredb.ErrorCodeWrongExpectedVersion {
				return fmt.Errorf("account %s was modified concurrently, please retry: %w", a.Id, err)
			}
			return err
		}
		return nil
	}

	delta := current.Balance.Current - a.Balance.Current
	if delta <= 0 {
		return nil
	}

	payload, err := json.Marshal(accountWithdrawPayload{
		Amount:    delta,
		Version:   &a.Version,
		UpdatedAt: &a.UpdatedAt,
	})
	if err != nil {
		return err
	}

	_, err = r.client.AppendToStream(
		ctx,
		stream,
		eventstoredb.AppendToStreamOptions{
			StreamState: eventstoredb.StreamRevision{Value: currentRevision},
		}, eventstoredb.EventData{
			ContentType: eventstoredb.ContentTypeJson,
			EventType:   EventAccountWithdraw,
			Data:        payload,
		})
	if err != nil {
		var eventStoreErr *eventstoredb.Error
		if errors.As(err, &eventStoreErr) && eventStoreErr.Code() == eventstoredb.ErrorCodeWrongExpectedVersion {
			return fmt.Errorf("account %s was modified concurrently, please retry: %w", a.Id, err)
		}
		return err
	}
	return err
}

func kycSessionAdded(current, next *accountdomain.UserAccount) bool {
	return len(next.KYCSessions) > len(current.KYCSessions)
}

// FindByID dựng lại trạng thái hiện tại bằng cách đọc và replay TOÀN BỘ event trong stream.
func (r *AccountRepository) FindByID(
	ctx context.Context,
	id string,
) (*accountdomain.UserAccount, error) {
	acc, _, err := r.findByIDWithRevision(ctx, id)
	return acc, err
}

// findByIDWithRevision làm y hệt FindByID, nhưng trả thêm revision của event CUỐI CÙNG đã đọc được - dùng để kiểm tra tranh chấp (optimistic
// concurrency) khi Save() ghi tiếp vào cùng stream này.
func (r *AccountRepository) findByIDWithRevision(
	ctx context.Context,
	id string,
) (*accountdomain.UserAccount, uint64, error) {
	const pageSize = PAGESIZE

	acc := &accountdomain.UserAccount{}
	found := false
	var lastRevision uint64

	opts := eventstoredb.ReadStreamOptions{}

	for {
		stream, err := r.client.ReadStream(
			ctx,
			streamName(id), opts, pageSize)
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
				var eventStoreErr *eventstoredb.Error
				if errors.As(err, &eventStoreErr) && eventStoreErr.Code() == eventstoredb.ErrorCodeResourceNotFound {
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
			metadata := replayEventMetadata{
				Revision:  event.Event.EventNumber,
				CreatedAt: event.Event.CreatedDate,
			}
			if err := applyEvent(acc, event.Event.EventType, metadata, decode); err != nil {
				stream.Close()
				return nil, 0, err
			}
		}
		stream.Close()

		if eventsInPage < pageSize {
			break
		}

		opts = eventstoredb.ReadStreamOptions{From: eventstoredb.Revision(lastRevision + 1)}
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

// FindByUsername cũng quét toàn bộ account stream; production nên dùng
// read-model/projection có index theo username.
func (r *AccountRepository) FindByUsername(ctx context.Context, username string) (*accountdomain.UserAccount, error) {
	accounts, err := r.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		if acc.Username == username {
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
	opts := eventstoredb.ReadAllOptions{} // trang đầu tiên: đọc từ Start

	for {
		all, err := r.client.ReadAll(ctx, opts, pageSize)
		if err != nil {
			return nil, err
		}

		eventsInPage := 0
		var lastPosition eventstoredb.Position

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

			if event.Event.EventType == EventAccountCreated {
				var p accountCreatedPayload
				if err := json.Unmarshal(event.Event.Data, &p); err != nil {
					all.Close()
					return nil, err
				}
				// ReadAll với From: Position là inclusive, nên event tại
				// lastPosition của trang trước sẽ được trả lại lần nữa ở
				// đầu trang sau -> dedupe theo id để tránh account bị lặp.
				account := upcastAccountCreated(p, replayEventMetadata{
					Revision:  event.Event.EventNumber,
					CreatedAt: event.Event.CreatedDate,
				})
				if _, dup := seen[account.Id]; !dup {
					seen[account.Id] = struct{}{}
					ids = append(ids, account.Id)
				}
			}
		}
		all.Close()

		if eventsInPage < pageSize {
			break
		}

		// From là inclusive nên trang kế tiếp đọc lại đúng event tại
		// lastPosition; dedupe ở trên xử lý phần trùng đó.
		opts = eventstoredb.ReadAllOptions{From: lastPosition}
	}

	return ids, nil
}
