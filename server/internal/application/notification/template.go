package notification

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dont-wait/anomaly/internal/domain/notification"
)

// RenderFunc biến Request.Params thành nội dung hiển thị.
//
//   - params: dữ liệu thô do mapper/caller truyền vào (đã được clone, sửa thoải mái).
//   - title, body: câu chữ tiếng Việt đã render, hiện trên app và banner.
//   - data: dữ liệu điều hướng cho mobile (vd "transferId"). Tối đa 20 key,
//     mỗi value ≤ 256 ký tự. Mobile tra route theo Type + data
//     (client/mobile/src/features/notifications/notificationRoutes.ts).
//   - err: thiếu param bắt buộc thì trả lỗi bọc ErrMissingParam (dùng
//     RequireParams). Đây là lỗi lập trình, unit test phải bắt được.
type RenderFunc func(params map[string]string) (title, body string, data map[string]string, err error)

// Template gắn một notification.Type với cách hiển thị của nó.
// Hướng dẫn tích hợp: docs/how-to-register-noti.md.
//
// File này chỉ là cơ chế đăng ký, không cần sửa khi thêm loại thông báo mới.
// Mỗi module tạo file templates_<module>.go trong package này và gọi Register
// trong init(), xem templates_system.go làm mẫu. Category và Severity do
// template quyết định, caller không truyền được, để cùng một Type luôn hiển
// thị nhất quán.
type Template struct {
	Type     notification.Type     // hằng số khai báo trong internal/domain/notification
	Category notification.Category // nhóm hiển thị/lọc: TRANSACTION, SECURITY, ...
	Severity notification.Severity // quyết định màu/icon: INFO, SUCCESS, WARNING, CRITICAL
	Render   RenderFunc
}

var (
	ErrMissingParam    = errors.New("missing template param")
	ErrUnknownTemplate = errors.New("no template registered for notification type")
)

var (
	templatesMu sync.RWMutex
	templates   = map[notification.Type]Template{}
)

// Register đăng ký template, chỉ gọi trong init().
//
// Panic khi template thiếu field hoặc Type đã được đăng ký: đây là lỗi lập
// trình nên phải vỡ ngay lúc khởi động/chạy test, không để lọt ra production.
func Register(t Template) {
	if t.Type == "" || t.Render == nil || !t.Category.Valid() || !t.Severity.Valid() {
		panic(fmt.Sprintf("notification: incomplete template for %q", t.Type))
	}
	templatesMu.Lock()
	defer templatesMu.Unlock()
	if _, exists := templates[t.Type]; exists {
		panic(fmt.Sprintf("notification: template %q registered twice", t.Type))
	}
	templates[t.Type] = t
}

func Lookup(t notification.Type) (Template, bool) {
	templatesMu.RLock()
	defer templatesMu.RUnlock()
	tmpl, ok := templates[t]
	return tmpl, ok
}

// RequireParams kiểm tra các param bắt buộc trong Render; key thiếu hoặc rỗng
// thì trả lỗi bọc ErrMissingParam kèm tên key.
func RequireParams(params map[string]string, keys ...string) error {
	for _, key := range keys {
		if params[key] == "" {
			return fmt.Errorf("%w: %s", ErrMissingParam, key)
		}
	}
	return nil
}
