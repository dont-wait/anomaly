package notification

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/notification"
)

// Request là yêu cầu phát một thông báo tới một người nhận.
type Request struct {
	// Audience: CUSTOMER cho khách hàng trên mobile (P0 chỉ dùng loại này).
	Audience notification.Audience
	// RecipientID: user id của người nhận, bằng JWT claims.UserID. Event banking
	// chỉ mang financial account id; tra user bằng AccountRepository.FindByFinancialID.
	RecipientID string
	// Type: phải có template đã Register, nếu không Notify trả ErrUnknownTemplate.
	Type notification.Type
	// SourceEventID: id ổn định của nguồn (event id trên EventStoreDB, assessment
	// id, ...). Cùng (SourceEventID, Type, RecipientID) thì cùng một thông báo, nên
	// retry hay replay không sinh bản trùng. Không dùng uuid ngẫu nhiên ở đây.
	SourceEventID string
	// OccurredAt: thời điểm nghiệp vụ xảy ra, không phải lúc gọi Notify. Event
	// cũ hơn NOTIFICATION_MAX_EVENT_AGE bị bỏ qua để replay không bắn tin cũ.
	OccurredAt time.Time
	// Params: input cho Template.Render của Type này.
	Params map[string]string
}

// Notifier là contract duy nhất module khác cần biết; lấy instance qua
// composition.NewNotifier. Hướng dẫn: docs/how-to-register-noti.md.
//
// Notify có thể nhận nhiều request một lần (vd cặp người gửi và người nhận) và
// ghi chung một lần. Lỗi trả về là để caller log và retry; tuyệt đối không để
// lỗi thông báo làm hỏng request nghiệp vụ (vd request chuyển tiền).
type Notifier interface {
	Notify(ctx context.Context, requests ...Request) error
}

type Store interface {
	InsertMany(ctx context.Context, items []notification.Notification) error
	List(ctx context.Context, audience notification.Audience, recipientID string, before *notification.Cursor, limit int, unreadOnly bool) ([]notification.Notification, error)
	ListAfter(ctx context.Context, audience notification.Audience, recipientID string, after notification.Cursor, limit int) ([]notification.Notification, error)
	CountUnread(ctx context.Context, audience notification.Audience, recipientID string) (int64, error)
	MarkRead(ctx context.Context, audience notification.Audience, recipientID, id string, at time.Time) error
	MarkAllRead(ctx context.Context, audience notification.Audience, recipientID string, before, at time.Time) (int64, error)
}

type Config struct {
	Retention   time.Duration
	MaxEventAge time.Duration
}

type Service struct {
	store  Store
	config Config
	now    func() time.Time
}

func NewService(store Store, config Config) *Service {
	if config.Retention <= 0 {
		config.Retention = 90 * 24 * time.Hour
	}
	return &Service{store: store, config: config, now: func() time.Time { return time.Now().UTC() }}
}

// Notify chạy theo thứ tự:
//  1. Bỏ qua request có OccurredAt cũ hơn MaxEventAge (không lỗi).
//  2. build: tra template theo Type, render, sinh ID xác định, gán
//     Category/Severity từ template, ExpiresAt = OccurredAt + Retention, validate.
//     Một request lỗi thì cả lô bị từ chối, không ghi gì.
//  3. Ghi tất cả bằng InsertMany ($setOnInsert): bản đã tồn tại giữ nguyên,
//     kể cả trạng thái đã đọc.
//
// Việc đẩy realtime không nằm ở đây: cmd/api theo dõi change stream của
// collection notifications rồi đẩy qua Hub, nên producer chạy ở process nào cũng được.
func (s *Service) Notify(ctx context.Context, requests ...Request) error {
	now := s.now()
	items := make([]notification.Notification, 0, len(requests))
	for _, request := range requests {
		if s.config.MaxEventAge > 0 && !request.OccurredAt.IsZero() && now.Sub(request.OccurredAt) > s.config.MaxEventAge {
			continue // replaying old history must not alert users
		}
		item, err := s.build(request, now)
		if err != nil {
			return err
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	return s.store.InsertMany(ctx, items)
}

func (s *Service) build(request Request, now time.Time) (notification.Notification, error) {
	if !request.Audience.Valid() || strings.TrimSpace(request.RecipientID) == "" || request.OccurredAt.IsZero() {
		return notification.Notification{}, fmt.Errorf("%w: audience, recipient and occurredAt are required", notification.ErrInvalid)
	}
	tmpl, ok := Lookup(request.Type)
	if !ok {
		return notification.Notification{}, fmt.Errorf("%w: %s", ErrUnknownTemplate, request.Type)
	}
	title, body, data, err := tmpl.Render(maps.Clone(request.Params))
	if err != nil {
		if errors.Is(err, ErrMissingParam) {
			return notification.Notification{}, fmt.Errorf("%w: %s: %w", notification.ErrInvalid, request.Type, err)
		}
		return notification.Notification{}, err
	}
	occurredAt := request.OccurredAt.UTC()
	item := notification.Notification{
		ID:            notification.NotificationID(request.SourceEventID, request.Type, request.RecipientID),
		Audience:      request.Audience,
		RecipientID:   request.RecipientID,
		Type:          request.Type,
		Category:      tmpl.Category,
		Severity:      tmpl.Severity,
		Title:         title,
		Body:          body,
		Data:          data,
		SourceEventID: request.SourceEventID,
		OccurredAt:    occurredAt,
		// Mongo stores milliseconds; truncating keeps cursors exact.
		CreatedAt: now.Truncate(time.Millisecond),
		ExpiresAt: occurredAt.Add(s.config.Retention),
	}
	if err := item.Validate(); err != nil {
		return notification.Notification{}, err
	}
	return item, nil
}
