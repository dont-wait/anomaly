package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	eventstoredb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	loanports "github.com/dont-wait/anomaly/internal/application/loan/ports"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

var ErrLoanInvalidReplay = errors.New("invalid loan replay")

// LoanRepository lưu hồ sơ vay bằng Event Sourcing.
// EventStoreDB là nguồn dữ liệu chính.
type LoanRepository struct {
	client *eventstoredb.Client
}

func NewLoanRepository(
	client *eventstoredb.Client,
) *LoanRepository {
	return &LoanRepository{
		client: client,
	}
}

// validateClient kiểm tra kết nối repository đã được cấu hình.
func (r *LoanRepository) validateClient() error {
	if r == nil || r.client == nil {
		return fmt.Errorf("loan eventstore client is not configured")
	}

	return nil
}

// Create tạo stream mới và ghi LoanApplicationCreated.
//
// NoStream đảm bảo không ghi đè hồ sơ đã tồn tại.
func (r *LoanRepository) Create(
	ctx context.Context,
	app *loandomain.Application,
) error {
	if err := r.validateClient(); err != nil {
		return err
	}

	if app == nil || strings.TrimSpace(app.Id) == "" {
		return fmt.Errorf("loan application ID is required")
	}

	if app.Status != loandomain.ApplicationStatusDraft {
		return fmt.Errorf("new loan application must be draft")
	}

	if len(app.Documents) != 0 {
		return fmt.Errorf("new loan application must not contain documents")
	}

	if app.CreatedAt.IsZero() || app.UpdatedAt.IsZero() {
		return fmt.Errorf("loan creation timestamps are required")
	}

	data, err := json.Marshal(
		loanApplicationCreatedPayload{
			Application: app,
		},
	)
	if err != nil {
		return err
	}
	preview := &loandomain.Application{}

	if err := applyLoanEvent(
		preview,
		EventLoanApplicationCreated,
		data,
	); err != nil {
		return err
	}

	_, err = r.client.AppendToStream(
		ctx,
		loanStreamName(app.Id),
		eventstoredb.AppendToStreamOptions{
			StreamState: eventstoredb.NoStream{},
		},
		eventstoredb.EventData{
			ContentType: eventstoredb.ContentTypeJson,
			EventType:   EventLoanApplicationCreated,
			Data:        data,
		},
	)
	if err != nil {
		return wrapLoanAppendError(app.Id, err)
	}

	return nil
}

// FindByID đọc toàn bộ lịch sử và dựng lại hồ sơ vay.
//
// Không tìm thấy stream: trả về nil, nil.
func (r *LoanRepository) FindByID(
	ctx context.Context,
	applicationID string,
) (*loandomain.Application, error) {
	app, _, err := r.FindByIDWithRevision(
		ctx,
		applicationID,
	)

	return app, err
}

// FindByIDWithRevision vừa khôi phục hồ sơ,
// vừa trả revision cuối của stream.
//
// Revision được dùng khi ghi sự kiện mới
// để kiểm soát optimistic concurrency.
func (r *LoanRepository) FindByIDWithRevision(
	ctx context.Context,
	applicationID string,
) (*loandomain.Application, uint64, error) {
	if err := r.validateClient(); err != nil {
		return nil, 0, err
	}

	if strings.TrimSpace(applicationID) == "" {
		return nil, 0, fmt.Errorf("loan application ID is required")
	}

	app := &loandomain.Application{}

	found := false
	var lastRevision uint64

	const pageSize = PAGESIZE

	opts := eventstoredb.ReadStreamOptions{}

	for {
		stream, err := r.client.ReadStream(
			ctx,
			loanStreamName(applicationID),
			opts,
			pageSize,
		)
		if err != nil {
			if isLoanStreamNotFound(err) && !found {
				return nil, 0, nil
			}

			return nil, 0, err
		}

		eventsInPage := 0

		for {
			recorded, err := stream.Recv()

			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				stream.Close()

				if isLoanStreamNotFound(err) && !found {
					return nil, 0, nil
				}

				return nil, 0, err
			}

			eventsInPage++

			if err := applyLoanEvent(
				app,
				recorded.Event.EventType,
				recorded.Event.Data,
			); err != nil {
				stream.Close()

				return nil, 0, fmt.Errorf(
					"%w: replay loan event %s failed: %w",
					ErrLoanInvalidReplay,
					recorded.Event.EventType,
					err,
				)
			}

			found = true
			lastRevision = recorded.Event.EventNumber
		}

		stream.Close()

		if eventsInPage < pageSize {
			break
		}

		// Tiếp tục từ sự kiện kế tiếp.
		opts = eventstoredb.ReadStreamOptions{
			From: eventstoredb.Revision(
				lastRevision + 1,
			),
		}
	}

	if !found {
		return nil, 0, nil
	}

	if app.Id != applicationID {
		return nil, 0, fmt.Errorf(
			"%w: loan stream ID does not match application ID",
			ErrLoanInvalidReplay,
		)
	}

	return app, lastRevision, nil
}

// Dùng chung kiểu Event với Application Layer.
type LoanAppendEvent = loanports.LoanAppendEvent

// Append giữ nguyên API ghi một sự kiện.
//
// Bên trong sử dụng AppendBatch để mọi thao tác
// ghi sự kiện đều đi qua cùng một quy trình kiểm tra.
func (r *LoanRepository) Append(
	ctx context.Context,
	applicationID string,
	expectedRevision uint64,
	eventType string,
	payload any,
) error {
	return r.AppendBatch(
		ctx,
		applicationID,
		expectedRevision,
		LoanAppendEvent{
			EventType: eventType,
			Payload:   payload,
		},
	)
}

// AppendBatch ghi nhiều sự kiện vào cùng một stream
// bằng một lần AppendToStream.
//
// Tất cả sự kiện phải hợp lệ và được kiểm tra
// theo đúng thứ tự trước khi ghi.
//
// Nếu một sự kiện không hợp lệ, không ghi sự kiện nào.
func (r *LoanRepository) AppendBatch(
	ctx context.Context,
	applicationID string,
	expectedRevision uint64,
	events ...LoanAppendEvent,
) error {
	if err := r.validateClient(); err != nil {
		return err
	}

	if strings.TrimSpace(applicationID) == "" {
		return fmt.Errorf("loan application ID is required")
	}

	if len(events) == 0 {
		return fmt.Errorf("at least one loan event is required")
	}

	// 1. Đọc lại hồ sơ và revision hiện tại.
	current, revision, err := r.FindByIDWithRevision(
		ctx,
		applicationID,
	)
	if err != nil {
		return err
	}

	if current == nil {
		return fmt.Errorf(
			"loan application %s does not exist",
			applicationID,
		)
	}

	// 2. Kiểm tra optimistic concurrency.
	if revision != expectedRevision {
		return fmt.Errorf(
			"loan application %s revision conflict: expected %d, current %d",
			applicationID,
			expectedRevision,
			revision,
		)
	}

	// 3. Chuẩn bị toàn bộ EventData.
	eventData := make(
		[]eventstoredb.EventData,
		0,
		len(events),
	)

	for index, event := range events {
		// Không cho tạo hồ sơ lần thứ hai hoặc ghi
		// các Event Type không được hỗ trợ.
		switch event.EventType {
		case EventLoanApplicationCreated,
			EventLoanDocumentAdded,
			EventLoanApplicationSubmitted,
			EventLoanReviewStarted,
			EventLoanApplicationAssessed,
			EventLoanApplicationApproved,
			EventLoanApplicationRejected,
			EventLoanDisbursementPrepared,
			EventLoanDisbursed:
		default:
			return fmt.Errorf(
				"unsupported loan event at index %d: %s",
				index,
				event.EventType,
			)
		}

		if event.Payload == nil {
			return fmt.Errorf(
				"loan event payload at index %d is required",
				index,
			)
		}

		data, err := json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf(
				"marshal loan event %d failed: %w",
				index,
				err,
			)
		}

		if string(data) == "null" {
			return fmt.Errorf(
				"loan event payload at index %d cannot be null",
				index,
			)
		}

		// 4. Kiểm tra từng sự kiện trên trạng thái
		// đang được dựng lại trong bộ nhớ.
		//
		// Ví dụ:
		// Event 1: Assessed -> cập nhật current.
		// Event 2: Approved -> kiểm tra trên current
		// đã có kết quả chấm điểm.
		if err := applyLoanEvent(
			current,
			event.EventType,
			data,
		); err != nil {
			return fmt.Errorf(
				"invalid loan event %d (%s): %w",
				index,
				event.EventType,
				err,
			)
		}

		eventData = append(
			eventData,
			eventstoredb.EventData{
				ContentType: eventstoredb.ContentTypeJson,
				EventType:   event.EventType,
				Data:        data,
			},
		)
	}

	// 5. Toàn bộ sự kiện đã hợp lệ.
	// Ghi cùng lúc vào một stream.
	//
	// EventStoreDB kiểm tra expectedRevision tại
	// thời điểm ghi để ngăn xung đột đồng thời.
	_, err = r.client.AppendToStream(
		ctx,
		loanStreamName(applicationID),
		eventstoredb.AppendToStreamOptions{
			StreamState: eventstoredb.StreamRevision{
				Value: expectedRevision,
			},
		},
		eventData...,
	)
	if err != nil {
		return wrapLoanAppendError(
			applicationID,
			err,
		)
	}

	return nil
}

// Nhận biết stream không tồn tại.
func isLoanStreamNotFound(err error) bool {
	var esErr *eventstoredb.Error

	return errors.As(err, &esErr) &&
		esErr.Code() == eventstoredb.ErrorCodeResourceNotFound
}

// Chuẩn hóa lỗi ghi sự kiện, đặc biệt là xung đột revision.
func wrapLoanAppendError(
	applicationID string,
	err error,
) error {
	var esErr *eventstoredb.Error

	if errors.As(err, &esErr) &&
		esErr.Code() == eventstoredb.ErrorCodeWrongExpectedVersion {
		return fmt.Errorf(
			"loan application %s was modified concurrently: %w",
			applicationID,
			err,
		)
	}

	return fmt.Errorf(
		"append loan event for application %s failed: %w",
		applicationID,
		err,
	)
}
