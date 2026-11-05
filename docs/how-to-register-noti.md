# Tích hợp thông báo cho một module

Hướng dẫn cho maintainer muốn module của mình (transaction, risk, account, loan, savings, ...) gửi thông báo tới khách hàng. Thiết kế đầy đủ nằm ở [`specs/noti-base.md`](../specs/noti-base.md).

## Tổng quan

```
             cách 1: event banking                cách 2: gọi trực tiếp
EventStoreDB ── cmd/notifier ── EventMapper        module (vd risk) ── Notifier.Notify
                       │                                   │
                       └──────────► Notifier.Notify ◄──────┘
                                         │ render template, ghi idempotent
                                         ▼
                              Mongo collection `notifications`
                                         │ change stream
                                         ▼
                             cmd/api Hub ── SSE ── mobile (badge, banner, trung tâm thông báo)
```

Module nghiệp vụ chỉ làm 3 việc:

1. Khai báo `Type` và **template** (câu chữ hiển thị).
2. **Phát** thông báo: viết `EventMapper` (cách 1) hoặc gọi `Notifier.Notify` (cách 2).
3. (Tùy chọn) Đăng ký **điều hướng** trên mobile khi người dùng bấm vào thông báo.

Store, HTTP API, SSE và màn hình trung tâm thông báo dùng chung, không cần sửa.

## Chọn cách phát

| Nguồn thông báo                       | Cách                              | Ví dụ                                                                          |
| ------------------------------------- | --------------------------------- | ------------------------------------------------------------------------------ |
| Event đã commit trên stream `banking` | **Cách 1: `EventMapper`**         | `TransferCompleted`, `TransferFailed`, giải ngân khoản vay, tất toán tiết kiệm |
| Không có event banking tương ứng      | **Cách 2: gọi `Notifier.Notify`** | Risk assessment `WARNING`, job nhắc hạn trả nợ                                 |

Ưu tiên cách 1 khi có event: thông báo chỉ phát khi giao dịch đã commit, `cmd/notifier` lo checkpoint và retry. **Không** gọi `Notify` từ HTTP handler trước khi lệnh được ghi vào EventStoreDB.

## Bước 1: khai báo Type

Thêm hằng số vào `server/internal/domain/notification/notification.go`:

```go
const (
    // ...
    TypeLoanDisbursed Type = "LOAN_DISBURSED"
)
```

Một số `Type` dự phòng đã có sẵn (`ACCOUNT_LOCKED`, `LOAN_APPROVED`, `LOAN_DISBURSED`, `LOAN_REPAYMENT_DUE`, `SAVINGS_MATURED`, ...). Có rồi thì dùng luôn.

## Bước 2: viết template

Tạo file `server/internal/application/notification/templates_<module>.go`, copy từ [`templates_system.go`](../server/internal/application/notification/templates_system.go):

```go
package notification

import "github.com/dont-wait/anomaly/internal/domain/notification"

func init() {
    Register(Template{
        Type:     notification.TypeLoanDisbursed,
        Category: notification.CategoryLoan,
        Severity: notification.SeveritySuccess,
        Render: func(p map[string]string) (string, string, map[string]string, error) {
            if err := RequireParams(p, "loanId", "amount", "last4"); err != nil {
                return "", "", nil, err
            }
            title := "Giải ngân thành công"
            body := "+" + p["amount"] + " VND vào tài khoản •••• " + p["last4"]
            data := map[string]string{"loanId": p["loanId"], "amount": p["amount"], "currency": "VND"}
            return title, body, data, nil
        },
    })
}
```

Quy tắc:

- **Category** dùng để nhóm và lọc: `TRANSACTION`, `SECURITY`, `ACCOUNT`, `LOAN`, `SAVINGS`, `SYSTEM`.
- **Severity** quyết định màu và icon trên mobile: `INFO`, `SUCCESS`, `WARNING`, `CRITICAL` (banner đỏ).
- `title` và `body` là tiếng Việt, render ở server.
- `data` chỉ chứa dữ liệu để điều hướng: tối đa 20 key, mỗi value ≤ 256 ký tự. Nhóm giao dịch tối thiểu có `transferId`, `direction`, `amount`, `currency`.
- **Không đưa dữ liệu nhạy cảm vào:** số tài khoản chỉ để dạng mask (`•••• 1234`). Không đưa OTP, risk score thô, model version hay lý do chặn chi tiết. Thông báo có thể hiện trên màn hình khóa.
- Thiếu param bắt buộc thì `RequireParams` trả lỗi. Đây là lỗi lập trình, phải có unit test bắt.
- Đăng ký trùng `Type` thì app panic ngay lúc khởi động.

## Bước 3a: phát từ event banking (EventMapper)

Viết mapper cạnh module sở hữu nghiệp vụ, implement interface trong [`dispatch.go`](../server/internal/application/notification/dispatch.go):

```go
type EventMapper interface {
    Supports(eventType string) bool
    Map(ctx context.Context, record journal.Record) ([]notification.Request, error)
}
```

Ví dụ khung:

```go
type LoanMapper struct{ accounts AccountLookup }

func (m LoanMapper) Supports(eventType string) bool {
    return eventType == journal.LoanDisbursementCredited
}

func (m LoanMapper) Map(ctx context.Context, r journal.Record) ([]notificationapp.Request, error) {
    owner, err := m.accounts.FindByFinancialID(ctx, r.AccountID)
    if err != nil {
        return nil, err // lỗi tạm thời: cmd/notifier sẽ retry
    }
    if owner == nil {
        return nil, nil // không có chủ trong hệ thống: không thông báo
    }
    return []notificationapp.Request{{
        Audience:      notification.AudienceCustomer,
        RecipientID:   owner.Id,
        Type:          notification.TypeLoanDisbursed,
        SourceEventID: r.ID,
        OccurredAt:    r.At,
        Params:        map[string]string{"loanId": r.LoanID, "amount": formatVND(r.LoanAmount), "last4": last4(r.AccountID)},
    }}, nil
}
```

Sau đó đăng ký trong `composition.NewNotificationDispatcher` ([`server/internal/composition/notification.go`](../server/internal/composition/notification.go)):

```go
mappers := []notificationapp.EventMapper{
    loannoti.NewMapper(mongo.NewAccountRepository(client, dbName)),
}
```

Lưu ý khi viết mapper:

- Event banking mang **financial account id**, không phải user id. Tra chủ tài khoản bằng `AccountRepository.FindByFinancialID`; `UserAccount.Id` chính là `RecipientID`.
- `SourceEventID = record.ID` và `OccurredAt = record.At`. Đừng dùng `time.Now()` hay uuid mới, nếu không replay sẽ sinh thông báo trùng.
- `Map` chỉ đọc, không ghi gì.
- Trả `nil, nil` khi event không cần thông báo, ví dụ tài khoản đích ngoài hệ thống.
- Trả lỗi khi tạm thời thiếu dữ liệu, ví dụ projection tài khoản chưa kịp cập nhật. `cmd/notifier` retry có backoff (tối đa 5 lần), quá số lần thì ghi `projection_failures` (id `notification:<eventId>`) rồi đi tiếp.
- Một event có thể sinh nhiều request. Ví dụ `TransferCompleted` tạo `TRANSFER_SENT` cho người gửi và `TRANSFER_RECEIVED` cho người nhận; hai cái khác `Type` và `RecipientID` nên không đè nhau.

## Bước 3b: gọi trực tiếp (Notifier)

Lấy instance qua `composition.NewNotifier(client, dbName, config.Notification)` rồi inject vào chỗ cần gọi. Chỉ gọi **sau khi** dữ liệu nghiệp vụ đã lưu xong:

```go
inserted, err := store.Save(ctx, assessment)
if err != nil {
    return err
}
if inserted && assessment.RiskLevel == "WARNING" {
    if err := notifier.Notify(ctx, notificationapp.Request{
        Audience:      notification.AudienceCustomer,
        RecipientID:   ownerUserID,
        Type:          notification.TypeSuspiciousActivity,
        SourceEventID: assessment.ID,
        OccurredAt:    assessment.EvaluatedAt,
        Params:        map[string]string{"time": formatTime(assessment.EvaluatedAt)},
    }); err != nil {
        log.Error().Err(err).Str("assessment_id", assessment.ID).Msg("notify suspicious activity failed")
        // KHÔNG trả lỗi ra ngoài: lỗi thông báo không được làm hỏng luồng nghiệp vụ.
    }
}
```

`SourceEventID` phải là id ổn định của nguồn, như ở đây là assessment id. Gọi lại bao nhiêu lần cũng chỉ ra một thông báo.

## Bước 4 (tùy chọn): điều hướng trên mobile

Khi bấm vào thông báo, mobile tra bảng trong [`client/mobile/src/features/notifications/notificationRoutes.ts`](../client/mobile/src/features/notifications/notificationRoutes.ts):

```ts
registerNotificationRoute("LOAN_DISBURSED", (data) =>
  data.loanId ? loanContract(data.loanId) : null,
);
```

- Key trong `data` phải khớp với `data` mà template server trả về.
- Không đăng ký thì mobile thử mở chi tiết giao dịch theo `data.transferId`, không có thì mở trung tâm thông báo. App bản cũ vẫn hiển thị được type mới, chỉ là không điều hướng.

## Kiểm thử

| Mức                | Nên có                                                                                                                                              |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Unit test template | Render đủ param ra đúng câu chữ; thiếu param trả `ErrMissingParam`                                                                                  |
| Unit test mapper   | Chỉ `Supports` đúng event; trả đúng recipient, `SourceEventID = record.ID`; trường hợp bỏ qua trả `nil, nil`                                        |
| E2E                | Tham khảo `server/internal/infrastructure/mongo/notification_e2e_test.go`: chạy cùng record 2 lần chỉ ra 1 thông báo (`make test-notification-e2e`) |
| Thử tay            | `make seed` tạo thông báo `SYSTEM_TEST` cho demo customer, rồi chạy `make test-notification-api` hoặc mở app mobile                                 |

## Chạy local

- `make start-api`: HTTP API, SSE và change stream đẩy realtime.
- `make start-notifier`: đọc stream `banking` và gọi các `EventMapper` (cách 1). Chỉ chạy **một instance**.
- Docker Compose đã có service `anomaly-notifier`.
- Lần đầu chạy, notifier bắt đầu từ **cuối** stream, không backfill lịch sử. Event cũ hơn `NOTIFICATION_MAX_EVENT_AGE` (mặc định 24h) bị bỏ qua.

| Biến                                    | Mặc định | Ý nghĩa                                              |
| --------------------------------------- | -------- | ---------------------------------------------------- |
| `NOTIFICATION_RETENTION_DAYS`           | `90`     | Thông báo tự xóa sau N ngày (TTL index)              |
| `NOTIFICATION_MAX_EVENT_AGE`            | `24h`    | Bỏ qua event cũ hơn ngưỡng, an toàn khi replay       |
| `NOTIFICATION_STREAM_MAX_CONN_PER_USER` | `5`      | Số kết nối SSE tối đa mỗi user trên một instance API |

## Checklist trước khi mở PR

- [ ] Hằng số `Type` trong `internal/domain/notification`
- [ ] `templates_<module>.go` với `Register` trong `init()`, có unit test
- [ ] Mapper đăng ký trong `composition.NewNotificationDispatcher` **hoặc** `Notify` gọi sau khi nghiệp vụ đã lưu
- [ ] `SourceEventID` ổn định, `OccurredAt` là thời điểm nghiệp vụ
- [ ] Lỗi `Notify` chỉ log hoặc retry, không trả về request nghiệp vụ
- [ ] Không có dữ liệu nhạy cảm trong title, body hay data
- [ ] (Nếu cần) route mobile trong `notificationRoutes.ts`
