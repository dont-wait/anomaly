# Spec — Thông báo khoản vay (giải ngân, trả nợ, nhắc hạn)

| Hạng mục | Chi tiết |
|----------|----------|
| **Mục tiêu** | Khách hàng nhận thông báo trong app khi tiền vay về tài khoản, khi thanh toán một kỳ thành công và khi sắp tới hạn trả nợ |
| **Phụ thuộc** | [`noti-base.md`](noti-base.md), [`noti-transfer.md`](noti-transfer.md) (cùng pattern `EventMapper`), `cmd/loan-reminder` (#203) |
| **Phạm vi** | `LOAN_DISBURSED`, `LOAN_INSTALLMENT_PAID` (mới), đóng nốt `LOAN_REPAYMENT_DUE` (triển khai chạy được), route mobile |
| **Ngoài phạm vi** | `LOAN_APPROVED`/rejected (nằm trên stream riêng của từng hồ sơ vay, notifier chưa đọc stream đó), kỳ quá hạn, phí phạt |
| **Trạng thái** | Draft, đã triển khai trên nhánh `feat/noti-loan` |
| **Ngày** | 2026-11-08 |

## 1. Nguồn sự kiện

| Thông báo | Nguồn | Cách phát |
|-----------|-------|-----------|
| `LOAN_DISBURSED` | `LoanDisbursementCredited` trên stream `banking` | `EventMapper` (cách 1) |
| `LOAN_INSTALLMENT_PAID` | `LoanInstallmentPaid` trên stream `banking` | `EventMapper` (cách 1) |
| `LOAN_REPAYMENT_DUE` | Quét lịch trả nợ (Mongo projection), `cmd/loan-reminder` | `Notifier.Notify` (cách 2), đã có từ #203 |

Hai thông báo đầu chạy trong `cmd/notifier` sẵn có. Nhắc hạn là process riêng.

## 2. Mapper (`internal/application/loan/notification`)

- Cả hai event mang **financial account id** của chính khách (`record.AccountID`). Tra chủ bằng `AccountRepository.FindByFinancialID`; `UserAccount.Id` là `RecipientID`.
- `SourceEventID = record.ID`, `OccurredAt = record.At`: replay không sinh bản trùng.
- Không tìm thấy tài khoản: trả lỗi để `cmd/notifier` retry rồi dead-letter. Lỗi DB: trả lỗi.
- `LoanInstallmentPaid` thiếu `LoanPayment`: bỏ qua (`nil, nil`).
- Helper dùng chung `FormatVND`, `Last4` chuyển sang `internal/application/notification/format.go`, mapper chuyển khoản dùng lại.

## 3. Template

Category `LOAN`, Severity `SUCCESS`. Không có số dư, số tài khoản đầy đủ hay mã hợp đồng trong câu chữ.

| Type | Title | Body | `data` |
|------|-------|------|--------|
| `LOAN_DISBURSED` | Giải ngân thành công | `+{amount} VND vào tài khoản •••• {last4}.` | `loanId`, `amount`, `currency` |
| `LOAN_INSTALLMENT_PAID` | Thanh toán khoản vay thành công | `-{amount} VND từ tài khoản •••• {last4} để thanh toán kỳ {installmentNo} của khoản vay.` | `loanId`, `amount`, `currency`, `installmentNo` |
| `LOAN_REPAYMENT_DUE` | Nhắc lịch thanh toán khoản vay | Có sẵn, không có số tiền hay ngày trong câu chữ | `loanId` |

`LoanInstallmentPaid` cũng có thể do chính người dùng bấm trả trên app; thông báo vẫn có ích như biên nhận.

## 4. Nhắc hẹn: phần đóng nốt

`cmd/loan-reminder` đã có logic T-5/T-0, dedupe theo ID xác định và chế độ dry-run, nhưng chưa chạy được cùng stack. Bổ sung:

| Hạng mục | Thay đổi |
|----------|----------|
| Docker image | Build và copy binary `/app/loan-reminder` trong `Dockerfile` |
| Compose | Service `anomaly-loan-reminder` (Mongo + migrate), truyền `LOAN_REMINDER_NOTIFICATIONS_ENABLED` (mặc định `false`) |
| Makefile | `make start-loan-reminder` |
| Bật thật | Đặt `LOAN_REMINDER_NOTIFICATIONS_ENABLED=true` sau khi migration thông báo đã áp dụng (process tự kiểm tra `CheckReady` và dừng nếu thiếu schema) |

Mặc định vẫn là dry-run (chỉ log, không ghi thông báo) để không bắn tin ngoài ý muốn khi deploy. Khi bật, chỉ quét trong khung 09:00 đến 21:00 giờ Việt Nam, tối đa một lần cho mỗi buổi sáng/chiều; chạy lại giữa chừng vẫn an toàn nhờ ID xác định.

## 5. Mobile

Route `LOAN_DISBURSED` và `LOAN_INSTALLMENT_PAID` mở hợp đồng vay theo `data.loanId` (giống `LOAN_REPAYMENT_DUE`); thiếu `loanId` thì mở trung tâm thông báo.

## 6. Kiểm thử

| Mức | Nội dung |
|-----|----------|
| Unit template + mapper | Câu chữ, `data`, không lộ số tài khoản đầy đủ, recipient đúng, `SourceEventID`, replay cùng ID, thiếu tài khoản/lỗi DB trả lỗi, thiếu payment thì bỏ qua |
| Unit helper | `FormatVND`, `Last4` |
| Vitest mobile | Hai type mới điều hướng đúng, thiếu `loanId` rơi về trung tâm thông báo |
| Thử tay | Giải ngân một khoản vay và trả một kỳ với `make start-notifier`; nhắc hạn: `make start-loan-reminder` với `LOAN_REMINDER_NOTIFICATIONS_ENABLED=true` |

## 7. Câu hỏi mở

1. `LOAN_APPROVED`/`LOAN_REJECTED`: cần notifier đọc thêm stream từng hồ sơ vay hoặc projector gọi `Notify`. Tách PR riêng.
2. Kỳ quá hạn nặng (sau 3 ngày catch-up) hiện không nhắc nữa; có cần thông báo `CRITICAL` riêng không?
