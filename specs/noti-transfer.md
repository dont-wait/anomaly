# Spec — Thông báo chuyển khoản thành công (người gửi và người nhận)

| Hạng mục | Chi tiết |
|----------|----------|
| **Mục tiêu** | Khi một lệnh chuyển khoản hoàn tất, cả người gửi lẫn người nhận đều nhận được thông báo trong app (lịch sử + realtime) |
| **Phụ thuộc** | Nền thông báo: [`noti-base.md`](noti-base.md), hướng dẫn [`docs/how-to-register-noti.md`](../docs/how-to-register-noti.md) (cách 1: `EventMapper`) |
| **Phạm vi** | Template `TRANSFER_SENT`, `TRANSFER_RECEIVED`; mapper cho `TransferCompleted`; đăng ký vào `cmd/notifier`; test |
| **Ngoài phạm vi** | `TRANSFER_BLOCKED`, `SUSPICIOUS_ACTIVITY`, push OS khi app tắt, email (vẫn do `receipt_outbox`) |
| **Trạng thái** | Draft, đã triển khai trên nhánh `feat/noti-transaction` |
| **Ngày** | 2026-11-07 |

---

## 1. Luồng

```
confirm OTP ─► EventStoreDB `banking`: TransferCompleted
                    │ subscription (checkpoint riêng)
                    ▼
              cmd/notifier ─► Dispatcher ─► transfernoti.Mapper
                    │                           │ tra chủ 2 tài khoản + tên đối tác
                    ▼                           ▼
              Notifier.Notify(sent, received)  (ghi chung 1 lần, idempotent)
                    ▼
              Mongo `notifications` ─ change stream ─► cmd/api Hub ─► SSE ─► mobile
```

Thông báo chỉ phát sau khi event đã commit, không gọi từ HTTP handler. Mobile đã đăng ký route `TRANSFER_SENT`/`TRANSFER_RECEIVED` → chi tiết giao dịch theo `data.transferId`, nên không cần sửa client.

## 2. Mapper `TransferCompleted`

Vị trí: `internal/application/transaction/notification` (package `transfernoti`).

| Mục | Quy tắc |
|-----|---------|
| `Supports` | Chỉ `journal.Completed` (`TransferCompleted`) |
| Nguồn dữ liệu | `record.Event.Payload` (`SourceAccountID`, `DestinationAccountID`, `Amount`, `Currency`), `record.Event.TransactionID` |
| Tra chủ tài khoản | `AccountRepository.FindByFinancialID` cho cả hai id. `UserAccount.Id` là `RecipientID`; `AccountNo` cho 4 số cuối |
| Tên đối tác | `CustomerRepository.FindByID(account.CustomerId).Profile.FullName`. Thiếu tên thì dùng "tài khoản •••• xxxx" |
| `SourceEventID` | `record.ID` |
| `OccurredAt` | `record.At` |
| Người gửi | `TRANSFER_SENT`, `counterparty` = người nhận |
| Người nhận | `TRANSFER_RECEIVED`, `counterparty` = người gửi |

Xử lý thiếu dữ liệu:

| Tình huống | Hành vi |
|------------|---------|
| Lỗi đọc DB | Trả lỗi, `cmd/notifier` retry (tối đa 5 lần, backoff) rồi dead-letter |
| Không tìm thấy tài khoản **nguồn** hoặc **đích** | Trả lỗi (chuyển khoản chỉ tới tài khoản nội bộ, nên thiếu là projection chưa kịp; retry, không gửi lẻ một bên) |
| Người gửi trùng người nhận | Không xảy ra (`Event.Validate` chặn); nếu có thì vẫn ra 2 thông báo khác `Type` |

Idempotent: ID thông báo = hash(`record.ID`, `Type`, `RecipientID`). Replay hoặc retry không sinh bản trùng, bản đã đọc giữ nguyên trạng thái.

## 3. Template

Đặt trong `internal/application/notification/templates_transfer.go`. Category `TRANSACTION`, Severity `SUCCESS`.

| Type | Title | Body |
|------|-------|------|
| `TRANSFER_SENT` | Chuyển tiền thành công | `-{amount} VND tới {counterpartyName} (•••• {last4}).` |
| `TRANSFER_RECEIVED` | Nhận tiền thành công | `+{amount} VND từ {counterpartyName} (•••• {last4}).` |

Param bắt buộc: `transferId`, `amount` (đã format `1.500.000`), `amountRaw` (số nguyên thô), `counterpartyName`, `last4`.

`data` (điều hướng): `transferId`, `direction` (`DEBIT` cho người gửi, `CREDIT` cho người nhận), `amount` (số nguyên thô, ví dụ `1500000`), `currency` = `VND`.

Quyết định về dữ liệu nhạy cảm (trả lời câu hỏi mở 3 của `noti-base.md`):

- **Không** đưa số dư sau giao dịch vào body hoặc data: thông báo có thể hiện trên màn hình khóa. Số dư xem trong chi tiết giao dịch.
- Số tài khoản chỉ ở dạng mask 4 số cuối. Không đưa OTP, risk score, nội dung chuyển khoản.
- Tên đối tác có trong body theo đúng `noti-base.md` mục 4.2.

## 4. Đăng ký

`composition.NewNotificationDispatcher` thêm `transfernoti.NewMapper(mongo.NewAccountRepository(...), mongo.NewCustomerRepository(...))`. Không cần đổi cấu hình, migration, API hay client.

## 5. Kiểm thử

| Mức | Nội dung |
|-----|----------|
| Unit template | Render đúng chữ, dấu +/-, `data` đủ khóa; thiếu param trả `ErrMissingParam`; body không chứa số dư hay số tài khoản đầy đủ |
| Unit mapper | `Supports` đúng event; ra 2 request đúng recipient/type/`SourceEventID`/`OccurredAt`; nguồn hoặc đích không có, hoặc lỗi DB, đều trả lỗi; thiếu tên dùng fallback; event khác bị bỏ qua |
| Idempotent | Map cùng record 2 lần cho cùng `SourceEventID`, nên `NotificationID` giống nhau |
| Thử tay | Chạy `make start-api`, `make start-notifier`, chuyển khoản giữa 2 tài khoản seed; cả hai app mobile thấy chuông tăng và banner |

## 6. Câu hỏi mở

1. Có muốn hiển thị số dư sau giao dịch cho người gửi không (đang tắt vì lý do màn hình khóa)? Nếu bật, nên làm tùy chọn ẩn/hiện ở phần preferences.
2. Có đưa nội dung chuyển khoản (memo) vào body không? Event hiện không mang trường này.
