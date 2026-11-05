package notification

import "github.com/dont-wait/anomaly/internal/domain/notification"

// Template mẫu: copy file này thành templates_<module>.go khi thêm thông báo
// cho module của bạn (xem docs/how-to-register-noti.md). SYSTEM_TEST cũng được
// seeder dev dùng để kiểm tra luồng end to end khi chưa có module nào phát
// thông báo thật.
func init() {
	Register(Template{
		// 1. Type: thêm hằng số trong internal/domain/notification/notification.go.
		Type: notification.TypeSystemTest,
		// 2. Category + Severity: quyết định nhóm và màu/icon trên mobile.
		Category: notification.CategorySystem,
		Severity: notification.SeverityInfo,
		// 3. Render: kiểm tra param bắt buộc, rồi trả title, body và data điều hướng.
		Render: func(params map[string]string) (string, string, map[string]string, error) {
			if err := RequireParams(params, "message"); err != nil {
				return "", "", nil, err
			}
			// data rỗng: mobile không có màn chi tiết riêng nên mở trung tâm thông báo.
			// Thông báo giao dịch thì trả {"transferId": ..., "direction": ..., ...}.
			return "Thông báo thử nghiệm", params["message"], map[string]string{}, nil
		},
	})
}
