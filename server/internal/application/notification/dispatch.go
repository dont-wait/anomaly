package notification

import (
	"context"
	"fmt"

	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

// EventMapper chuyển một record trên stream `banking` thành các Request.
// Hướng dẫn: docs/how-to-register-noti.md (cách 1).
//
// Dùng khi thông báo bắt nguồn từ event đã commit (TransferCompleted,
// TransferFailed, ...). Mapper đặt cạnh module sở hữu nghiệp vụ và được đăng ký
// trong composition.NewNotificationDispatcher; package này không import module
// nghiệp vụ.
//
// Quy ước khi viết mapper:
//   - Supports: trả true chỉ cho các event type mapper xử lý; phải rẻ, không I/O.
//   - Map: chỉ đọc dữ liệu, không ghi gì. Dùng record.ID làm SourceEventID và
//     thời điểm của event (record.At) làm OccurredAt.
//   - Trả nil, nil khi event không cần thông báo (vd tài khoản đích ngoài hệ thống).
//   - Trả lỗi khi tạm thời thiếu dữ liệu (vd projection tài khoản chưa có):
//     cmd/notifier retry có backoff, quá số lần thì ghi projection_failures rồi đi tiếp.
type EventMapper interface {
	Supports(eventType string) bool
	Map(ctx context.Context, record journal.Record) ([]Request, error)
}

// Dispatcher gom request từ mọi mapper hỗ trợ event rồi gọi Notify một lần.
// cmd/notifier gọi Handle cho từng event và chỉ lưu checkpoint khi Handle thành công.
type Dispatcher struct {
	mappers  []EventMapper
	notifier Notifier
}

func NewDispatcher(notifier Notifier, mappers ...EventMapper) *Dispatcher {
	return &Dispatcher{notifier: notifier, mappers: mappers}
}

// Handle gọi lại cho cùng một record vẫn an toàn (crash giữa chừng rồi chạy
// lại event): ID thông báo xác định nên không sinh bản trùng.
func (d *Dispatcher) Handle(ctx context.Context, record journal.Record) error {
	var requests []Request
	for _, mapper := range d.mappers {
		if !mapper.Supports(record.Type) {
			continue
		}
		mapped, err := mapper.Map(ctx, record)
		if err != nil {
			return fmt.Errorf("map %s %s: %w", record.Type, record.ID, err)
		}
		requests = append(requests, mapped...)
	}
	if len(requests) == 0 {
		return nil
	}
	return d.notifier.Notify(ctx, requests...)
}
