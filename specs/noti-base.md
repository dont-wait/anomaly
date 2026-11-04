# Spec — Notification Base

| Hạng mục | Chi tiết |
|----------|----------|
| **Mục tiêu** | Dựng nền tảng thông báo dùng chung để các module (transaction, risk, account, loan, savings) gọi lại |
| **Phạm vi** | Domain + application + Mongo store + API đọc/đánh dấu đã đọc + SSE realtime + notification center trên mobile |
| **Đối tượng nhận** | Customer (mobile) trong P0; admin (desktop) thiết kế sẵn qua `audience`, triển khai sau |
| **Trạng thái** | Draft |
| **Ngày** | 2026-11-04 |

---

## 1. Bối cảnh

Hiện trạng liên quan trong repo:

- `internal/application/riskstream.Hub`: hub SSE in-process, **broadcast tới mọi client**, chỉ phục vụ admin monitor (`GET /api/admin/monitor/stream`). Nguồn dữ liệu là Mongo change stream trên `risk_assessments`, nên chạy được với nhiều instance API.
- `receipt_outbox` + `ReceiptWorker`: email biên lai sau khi chuyển khoản thành công, có retry/backoff. Đây là kênh email riêng, **giữ nguyên**, không gộp vào base này.
- `client/mobile/src/shared/notifications`: chỉ có toast tạm thời (5s), không có lịch sử, không có realtime.
- Mobile chưa có plugin thông báo hệ điều hành (`Cargo.toml` mới có `tauri-plugin-opener`).

Các case cần phục vụ ngay sau khi có base:

| # | Case | Nguồn sự kiện |
|---|------|---------------|
| 1 | Chuyển khoản thành công (người gửi) | `TransferCompleted` trên stream `banking` |
| 2 | Nhận tiền thành công (người nhận) | `TransferCompleted` trên stream `banking` |
| 3 | Giao dịch bất thường bị chặn | `TransferFailed` có `RiskAssessment` (decision `BLOCK`, score > 0.7) |
| 4 | Hành vi đáng ngờ, risk trung bình | Risk assessment `riskLevel = WARNING` (0.5 ≤ score ≤ 0.7) |

Base **không** tự cài logic của các case trên; nó cung cấp contract để module tương ứng gọi. Mục 9 mô tả cách từng case gọi vào.

---

## 2. Nguyên tắc thiết kế

1. **Notification là read model, không phải nguồn sự thật.** Không bao giờ dùng notification để quyết định tiền hay trạng thái lệnh. Nội dung chỉ để hiển thị; chi tiết luôn mở qua API canonical (ví dụ chi tiết giao dịch).
2. **Idempotent theo khóa xác định.** Mỗi notification có `_id` sinh từ `(sourceEventId, type, recipientId)`. Replay sự kiện, retry hay chạy trùng worker đều không sinh bản trùng.
3. **Ghi trước, đẩy sau.** Producer chỉ ghi vào Mongo `notifications`. Việc đẩy realtime đi qua change stream, giống mô hình `risk_assessments` hiện có, nên không phụ thuộc instance nào đang giữ kết nối SSE.
4. **Lỗi notification không làm hỏng luồng nghiệp vụ.** Gọi `Notify` lỗi thì log và retry theo cơ chế của producer (checkpoint), không trả lỗi cho request chuyển tiền.
5. **Không lộ dữ liệu nhạy cảm.** Body chỉ chứa số tài khoản đã mask (`•••• 1234`), không chứa OTP, risk score thô, model version hay lý do chặn chi tiết cho customer.
6. **Đúng layering.** `domain` → `application` → `infrastructure` → `presentation/http`, nối ở `composition`. Application/HTTP không phụ thuộc lỗi Mongo; `composition` dịch lỗi adapter.

---

## 3. Domain — `internal/domain/notification`

### 3.1 Model

```go
type Notification struct {
    ID            string            // deterministic, xem 3.3
    Audience      Audience          // CUSTOMER | ADMIN
    RecipientID   string            // userId của customer; với ADMIN là role hoặc staff id
    Type          Type
    Category      Category
    Severity      Severity
    Title         string            // đã render, tiếng Việt
    Body          string            // đã render, tiếng Việt
    Data          map[string]string // dữ liệu điều hướng: transferId, accountNo (mask), amount...
    SourceEventID string            // event gốc trên EventStoreDB hoặc assessment id
    OccurredAt    time.Time         // thời điểm sự kiện nghiệp vụ
    CreatedAt     time.Time         // thời điểm ghi notification
    ReadAt        *time.Time
    ExpiresAt     time.Time         // TTL
}
```

### 3.2 Enum

| Enum | Giá trị |
|------|---------|
| `Audience` | `CUSTOMER`, `ADMIN` |
| `Category` | `TRANSACTION`, `SECURITY`, `ACCOUNT`, `LOAN`, `SAVINGS`, `SYSTEM` |
| `Severity` | `INFO`, `SUCCESS`, `WARNING`, `CRITICAL` |
| `Type` (P0) | `TRANSFER_SENT`, `TRANSFER_RECEIVED`, `TRANSFER_BLOCKED`, `SUSPICIOUS_ACTIVITY` |
| `Type` (dự phòng) | `ACCOUNT_LOCKED`, `ACCOUNT_UNLOCKED`, `LOAN_APPROVED`, `LOAN_DISBURSED`, `LOAN_REPAYMENT_DUE`, `SAVINGS_MATURED` |

Thêm `Type` mới = thêm hằng số + template (mục 4.2). Không cần sửa store, API hay client core.

### 3.3 Khóa idempotent

```go
func NotificationID(sourceEventID string, t Type, recipientID string) string {
    return uuid.NewSHA1(uuid.NameSpaceOID, []byte(sourceEventID+":"+string(t)+":"+recipientID)).String()
}
```

Theo cùng pattern với `journal.EventID`. Một `TransferCompleted` sinh 2 notification khác nhau (người gửi `TRANSFER_SENT`, người nhận `TRANSFER_RECEIVED`) vì khác `type` và `recipientId`.

### 3.4 Validate

- `ID`, `RecipientID`, `SourceEventID`, `Title` bắt buộc.
- `Type` phải có template đã đăng ký và `Category`/`Severity` mặc định khớp template.
- `Data` tối đa 20 key, mỗi value ≤ 256 ký tự.

---

## 4. Application — `internal/application/notification`

### 4.1 Contract cho module gọi

Đây là interface duy nhất các module khác cần biết:

```go
type Request struct {
    Audience      notification.Audience
    RecipientID   string
    Type          notification.Type
    SourceEventID string
    OccurredAt    time.Time
    Params        map[string]string // input cho template
}

type Notifier interface {
    Notify(ctx context.Context, requests ...Request) error
}
```

`Notify`:

1. Tra template theo `Type`, render `Title`/`Body`/`Data` từ `Params`.
2. Sinh `ID` theo 3.3, gán `Category`/`Severity` từ template, `ExpiresAt = OccurredAt + retention`.
3. Gọi `Store.Insert` dạng upsert `$setOnInsert`. Bản đã tồn tại thì bỏ qua, không lỗi.
4. Bỏ qua (không lỗi) nếu `OccurredAt` cũ hơn `NOTIFICATION_MAX_EVENT_AGE` để replay lịch sử không bắn thông báo cũ.

Một lần gọi có thể chứa nhiều request (ví dụ cặp người gửi và người nhận) và ghi bằng `BulkWrite` unordered.

### 4.2 Template registry

```go
type Template struct {
    Type     notification.Type
    Category notification.Category
    Severity notification.Severity
    Render   func(params map[string]string) (title, body string, data map[string]string, err error)
}

func Register(t Template) // gọi trong init() của file template từng module
```

Template đặt cạnh module sở hữu nghiệp vụ, ví dụ `internal/application/notification/templates_transfer.go`, đăng ký qua `init()` giống seeder. Thiếu param bắt buộc thì `Render` trả lỗi và `Notify` trả lỗi validate (lỗi lập trình, cần test bắt).

Template P0 (để tham chiếu, module tự hoàn thiện câu chữ):

| Type | Severity | Title | Body |
|------|----------|-------|------|
| `TRANSFER_SENT` | `SUCCESS` | Chuyển tiền thành công | `-{amount} VND tới {counterpartyName} (•••• {last4}). Số dư: {balanceAfter} VND` |
| `TRANSFER_RECEIVED` | `SUCCESS` | Nhận tiền thành công | `+{amount} VND từ {counterpartyName} (•••• {last4}). Số dư: {balanceAfter} VND` |
| `TRANSFER_BLOCKED` | `CRITICAL` | Giao dịch bị tạm chặn | `Giao dịch {amount} VND tới •••• {last4} đã bị chặn vì dấu hiệu bất thường. Liên hệ tổng đài nếu đây là bạn.` |
| `SUSPICIOUS_ACTIVITY` | `WARNING` | Phát hiện hoạt động bất thường | `Tài khoản có giao dịch cần lưu ý lúc {time}. Nếu không phải bạn, hãy đổi mật khẩu ngay.` |

`Data` tối thiểu cho nhóm transaction: `transferId`, `direction`, `amount`, `currency`. Mobile dùng `transferId` để mở màn chi tiết giao dịch.

### 4.3 Commands và queries

| Use case | Loại | Mô tả |
|----------|------|-------|
| `ListNotifications(recipient, cursor, limit, unreadOnly)` | query | Sắp xếp `created_at desc, _id desc`, cursor opaque base64 |
| `CountUnread(recipient)` | query | Dùng cho badge |
| `MarkRead(recipient, id)` | command | Chỉ chủ sở hữu, idempotent |
| `MarkAllRead(recipient, before)` | command | Đánh dấu tất cả notification `created_at <= before` |

Mọi query/command đều lọc theo `audience` + `recipient_id` lấy từ JWT claims. Không có API nào nhận `recipientId` từ client.

### 4.4 Realtime fan-out

Thêm `internal/application/notification.Hub`, khác `riskstream.Hub` ở chỗ **subscribe theo recipient**:

```go
func (h *Hub) Subscribe(recipientKey string) (<-chan notification.Notification, func())
func (h *Hub) Publish(n notification.Notification) // chỉ gửi tới subscriber cùng recipientKey
```

- `recipientKey = audience + ":" + recipientId`.
- Giữ hành vi của `riskstream.Hub`: buffer 32, client chậm thì đóng kênh để client reconnect và replay theo cursor, không âm thầm drop.
- Giới hạn tối đa 5 kết nối đồng thời mỗi recipient.

---

## 5. Infrastructure

### 5.1 Mongo collection `notifications`

Migration mới: `server/migrations/000020_create_notifications.{up,down}.json` (số tiếp theo sau `000019`; kiểm tra lại số khi merge để tránh trùng).

```json
{
  "_id": "string (deterministic)",
  "audience": "CUSTOMER|ADMIN",
  "recipient_id": "string",
  "type": "string",
  "category": "string",
  "severity": "INFO|SUCCESS|WARNING|CRITICAL",
  "title": "string",
  "body": "string",
  "data": { "transferId": "..." },
  "source_event_id": "string",
  "occurred_at": "date",
  "created_at": "date",
  "read_at": "date|null",
  "expires_at": "date"
}
```

Index:

| Tên | Key | Ghi chú |
|-----|-----|---------|
| `idx_notifications_inbox` | `{audience:1, recipient_id:1, created_at:-1, _id:-1}` | List + cursor |
| `idx_notifications_unread` | `{audience:1, recipient_id:1, read_at:1}` partial `read_at: null` | Badge |
| `idx_notifications_ttl` | `{expires_at:1}` `expireAfterSeconds: 0` | Retention mặc định 90 ngày |

Validator `$jsonSchema` strict như `receipt_outbox`. `EnsureIndexes` báo lỗi nếu collection chưa có (yêu cầu chạy migrate trước), giống `ReceiptOutboxRepository`.

### 5.2 Repository — `internal/infrastructure/mongo/notification_repository.go`

- `InsertMany(ctx, []Notification)`: upsert `$setOnInsert`, unordered.
- `List`, `CountUnread`, `MarkRead`, `MarkAllRead`.
- `ListAfter(ctx, recipientKey, cursor, limit)`: replay cho SSE.
- `Watch(ctx, resumeToken, startAt, fn)`: change stream `operationType: insert`, trả resume token. Tái sử dụng pattern retry/`isChangeStreamHistoryLost` trong `cmd/api/main.go`; nên tách pattern này thành helper dùng chung thay vì copy lần thứ ba.

### 5.3 Event consumer cho banking stream

Case 1–3 bắt nguồn từ stream `banking`. Cài một **catch-up subscription riêng với checkpoint riêng** (`notification-dispatcher`), không nhét vào transaction của `BankProjection.Apply`:

- Không ràng buộc vòng đời notification vào banking projection: `projector --replay` không sinh lại thông báo, lỗi notification không chặn projection.
- Lần đầu chạy (chưa có checkpoint) khởi tạo checkpoint ở **cuối stream**, không backfill lịch sử.
- Host trong `cmd/projector` như một goroutine riêng, bật bằng env `NOTIFICATION_DISPATCHER_ENABLED=true`. Tách thành `cmd/notifier` sau nếu cần scale riêng.
- Dispatcher chỉ map event sang `[]Request` rồi gọi `Notifier.Notify`; ghi checkpoint sau khi `Notify` thành công. Crash giữa chừng thì chạy lại event, idempotent nhờ 3.3.
- Tra `userId` của chủ tài khoản từ projection tài khoản trong Mongo (eventual consistency chấp nhận được vì không liên quan quyết định tiền). Không tìm thấy thì retry có backoff, quá N lần thì ghi `projection_failures` và đi tiếp.
- Tài khoản đích ngoài hệ thống (không có userId) thì bỏ qua phía người nhận.

### 5.4 Interface mapper cho module

```go
// internal/application/notification/dispatch.go
type EventMapper interface {
    Supports(eventType string) bool
    Map(ctx context.Context, record journal.Record) ([]Request, error)
}
```

Dispatcher giữ danh sách `EventMapper` được đăng ký ở `composition`. Module transaction cung cấp mapper cho `TransferCompleted`/`TransferFailed`; sau này module loan/savings/account chỉ việc đăng ký mapper riêng.

---

## 6. HTTP API — `internal/presentation/http/handler/notification`

Đăng ký qua `openapi.Registry` trong `notification_routes.go`, tag `Notifications`, tất cả `Auth: true` với `middleware.RequireAuth`. Envelope chuẩn `{status, message, data}` / `{status, title, errors}`.

| Method | Path | Mô tả |
|--------|------|-------|
| `GET` | `/api/notifications?cursor=&limit=20&unread=false` | Danh sách, `limit` 1–50 |
| `GET` | `/api/notifications/unread-count` | `{ "count": 3 }` |
| `PATCH` | `/api/notifications/{id}/read` | Đánh dấu đã đọc; id không thuộc user trả `404` (không lộ là tồn tại) |
| `POST` | `/api/notifications/read-all` | Body `{ "before": "<RFC3339>" }`, mặc định là now |
| `GET` | `/api/notifications/stream` | SSE, xem 6.2 |

### 6.1 DTO

```json
{
  "id": "6f1c…",
  "type": "TRANSFER_RECEIVED",
  "category": "TRANSACTION",
  "severity": "SUCCESS",
  "title": "Nhận tiền thành công",
  "body": "+500.000 VND từ NGUYEN VAN A (•••• 1234). Số dư: 2.300.000 VND",
  "data": { "transferId": "…", "direction": "CREDIT", "amount": "500000", "currency": "VND" },
  "occurredAt": "2026-11-04T08:00:00Z",
  "createdAt": "2026-11-04T08:00:01Z",
  "read": false
}
```

List trả thêm `nextCursor` (rỗng khi hết).

### 6.2 SSE

- Auth bằng header `Authorization: Bearer` và client đọc stream bằng `fetch` (như `client/desktop/src/features/monitor/api/monitorApi.ts`), vì `EventSource` không gửi được header.
- Replay theo `?cursor=` hoặc `Last-Event-ID` qua `ListAfter`, sau đó chuyển sang live từ hub. Xử lý trùng giữa replay và live bằng `id` ở client.
- Format:

  ```
  id: <cursor>
  event: notification
  data: <NotificationDTO JSON>
  ```

- Heartbeat `: ping` mỗi 15s. Dùng lại `setSSEWriteDeadline` và các header `no-cache`, `X-Accel-Buffering: no` của monitor handler.
- Event `unread-count` (`data: {"count": n}`) gửi khi mở kết nối và sau mỗi notification mới, để badge không cần poll.

---

## 7. Client mobile

Đặt tại `client/mobile/src/features/notifications/`. Base chỉ dựng hạ tầng, các màn nghiệp vụ không cần sửa.

| Thành phần | Mô tả |
|------------|-------|
| `api/notificationsApi.ts` | `list`, `unreadCount`, `markRead`, `markAllRead`, `openStream` (fetch + parser SSE, reconnect backoff 1s→30s, lưu `lastEventId` trong memory) |
| `NotificationProvider` | Mount một lần sau khi đăng nhập trong `src/app`; giữ kết nối stream, unread count, đóng stream khi logout hoặc token hết hạn |
| `useNotifications()` | Hook đọc list (phân trang vô hạn), count, các action |
| `NotificationBell` | Icon + badge trên header dashboard |
| `NotificationCenterPage` | Route `/notifications`: danh sách, lọc chưa đọc, "Đánh dấu tất cả đã đọc", bấm item thì `markRead` rồi điều hướng theo `data` (ví dụ `transferId` → chi tiết giao dịch) |
| `notificationRoutes.ts` | Bảng `type → (data) => route` để module đăng ký điều hướng; type không biết thì chỉ mở center |

Khi nhận notification mới lúc app đang mở: hiện in-app banner. Có thể dùng `shared/notifications/toast` nhưng toast hiện chỉ có `error | success`; cần mở rộng thêm tone `warning`/`info` và cho phép bấm vào banner.

**Thông báo hệ điều hành (P1):** thêm `tauri-plugin-notification` (Android + Linux) để hiện notification hệ thống khi app ở background nhưng process còn sống. Push thật khi app bị kill (FCM) **nằm ngoài scope**; khi cần sẽ thêm channel `push` và bảng device token, không đổi contract `Notifier`.

Desktop admin: P0 không làm. `audience=ADMIN` đã có trong model; khi triển khai thì thêm route `/api/admin/notifications*` dùng chung application layer.

---

## 8. Cấu hình

Thêm vào `server/.env.example`:

| Biến | Mặc định | Ý nghĩa |
|------|----------|---------|
| `NOTIFICATION_DISPATCHER_ENABLED` | `true` | Bật dispatcher trong `cmd/projector` |
| `NOTIFICATION_RETENTION_DAYS` | `90` | TTL |
| `NOTIFICATION_MAX_EVENT_AGE` | `24h` | Bỏ qua event cũ hơn ngưỡng (an toàn khi replay) |
| `NOTIFICATION_STREAM_MAX_CONN_PER_USER` | `5` | Giới hạn SSE |

---

## 9. Cách các module gọi vào base (ngoài scope base, làm ở PR riêng)

| Case | Module sở hữu | Điểm gọi | Recipient |
|------|---------------|----------|-----------|
| `TRANSFER_SENT` | transaction | `EventMapper` cho `TransferCompleted` | chủ `SourceAccountID` |
| `TRANSFER_RECEIVED` | transaction | Cùng mapper, cùng event | chủ `DestinationAccountID` (nếu là tài khoản nội bộ) |
| `TRANSFER_BLOCKED` | transaction / risk | `EventMapper` cho `TransferFailed` có `RiskAssessment` decision `BLOCK` | chủ `SourceAccountID` |
| `SUSPICIOUS_ACTIVITY` | risk | Nơi API lưu assessment sau khi verify chữ ký Kafka: nếu `riskLevel == WARNING` thì gọi `Notifier.Notify`, `SourceEventID = assessment id` | chủ tài khoản nguồn của giao dịch |
| `ACCOUNT_LOCKED` (sau) | account | `EventMapper` cho event đổi trạng thái tài khoản (#194) | chủ tài khoản |

Ghi chú:

- Case 1–3 đi qua event trên stream `banking`, nên chỉ phát khi giao dịch thật sự đã commit, không phát từ HTTP handler.
- Case 4 đi từ risk assessment, không có event banking tương ứng. Assessment được lưu idempotent theo id nên `Notify` cũng idempotent.
- Mỗi module tự đăng ký template trong `init()` và mapper trong `composition`, base không import module nghiệp vụ.

---

## 10. Kế hoạch triển khai base

1. Domain `notification` + `NotificationID` + validate + unit test.
2. Migration `000020_create_notifications` + `NotificationRepository` + `EnsureIndexes`.
3. Application: `Notifier`, template registry, queries/commands, `Hub` theo recipient.
4. Dispatcher banking stream (checkpoint riêng, khởi tạo ở cuối stream, `EventMapper` registry rỗng) trong `cmd/projector`.
5. Change stream `notifications` → `Hub` trong `cmd/api` (tách helper watch/retry dùng chung với risk).
6. HTTP handler + routes + SSE; `openapi.json` sinh tự động.
7. Mobile: api, provider, bell, center page, routing registry, mở rộng toast.
8. Một template mẫu `SYSTEM_TEST` chỉ bật trong `APP_ENV=development` + seeder để kiểm tra end-to-end khi chưa có module nào gọi.

---

## 11. Kiểm thử

| Mức | Nội dung |
|-----|----------|
| Unit (Go, `make test`) | ID xác định và khác nhau theo type/recipient; template thiếu param; `Notify` bỏ qua event quá cũ; hub chỉ gửi đúng recipient và đóng client chậm; cursor encode/decode |
| Handler | 401 khi thiếu token; user A không đọc/đánh dấu được notification của user B (404); `limit` ngoài khoảng trả 400 |
| E2E (`-tags=e2e`, Testcontainers) | Insert trùng không tạo bản thứ hai; TTL index tồn tại; change stream đẩy tới SSE; dispatcher replay cùng event 2 lần chỉ ra 1 notification |
| Hurl (`make test-api`) | Luồng list → unread-count → mark read → read-all |
| Vitest mobile | Parser SSE (chunk cắt giữa dòng, heartbeat), reconnect với `lastEventId`, badge cập nhật, điều hướng theo `type` |

---

## 12. Ngoài phạm vi

- Push FCM/APNs khi app bị kill, quản lý device token.
- Tùy chọn bật/tắt theo loại (preferences). Khi làm: `SECURITY` và `TRANSFER_BLOCKED` không được tắt.
- SMS. Email vẫn dùng `receipt_outbox` hiện tại.
- Notification center cho admin desktop.
- Đa ngôn ngữ (P0 chỉ tiếng Việt, render ở server).

---

## 13. Câu hỏi mở

1. `SUSPICIOUS_ACTIVITY` có báo cho customer không, hay chỉ báo admin? Báo customer giúp chống chiếm tài khoản nhưng có thể gây hoang mang nếu model báo nhầm nhiều. Đề xuất: P0 báo customer với câu chữ trung tính, kèm rate limit 1 lần mỗi 30 phút cho mỗi tài khoản.
2. `TRANSFER_BLOCKED` hiện được ghi ở bước confirm (`FailTransfer` với assessment). Có cần báo sớm hơn, ngay khi assessment `BLOCK` về, trước khi người dùng bấm confirm không?
3. Có hiển thị số dư sau giao dịch trong body không? Thông báo hệ điều hành có thể hiện trên màn hình khóa.
4. Dispatcher chạy trong `cmd/projector` hay tách `cmd/notifier` ngay từ đầu (thêm service vào Compose)?
