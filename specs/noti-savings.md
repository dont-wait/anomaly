# Spec — Thông báo tiết kiệm (gửi và tất toán)

| Hạng mục | Chi tiết |
|----------|----------|
| **Mục tiêu** | Khách hàng nhận thông báo trong app khi gửi tiết kiệm thành công và khi tất toán (đáo hạn) nhận gốc cộng lãi |
| **Phụ thuộc** | [`noti-base.md`](noti-base.md), [`noti-transfer.md`](noti-transfer.md), [`noti-loan.md`](noti-loan.md) (helper `FormatVND`, `Last4`) |
| **Phạm vi** | `SAVINGS_OPENED` (mới), `SAVINGS_MATURED` (có sẵn type), mapper, route mobile |
| **Ngoài phạm vi** | Nhắc sổ sắp đáo hạn (cần job quét như `loan-reminder`), rút trước hạn, màn chi tiết từng sổ trên mobile |
| **Trạng thái** | Draft, đã triển khai trên nhánh `feat/noti-savings` |
| **Ngày** | 2026-11-08 |

## 1. Nguồn sự kiện

Hai event trên stream `banking`, mỗi event gồm cả hợp đồng và biến động số dư trong một bản ghi (`SavingsMovement`), nên thông báo chỉ phát khi tiền đã thật sự đổi chỗ:

| Event | Ý nghĩa | Thông báo |
|-------|---------|-----------|
| `SavingsOpened` | Trừ tiền gốc khỏi tài khoản nguồn, mở sổ | `SAVINGS_OPENED` |
| `SavingsSettled` | Cộng gốc + lãi về tài khoản nguồn khi đáo hạn | `SAVINGS_MATURED` |

## 2. Mapper (`internal/application/savings/notification`)

- Chủ tài khoản: `FindByFinancialID(record.AccountID)`, `UserAccount.Id` là `RecipientID`.
- `SourceEventID = record.ID`, `OccurredAt = record.At`. Hai event của cùng một sổ có `record.ID` khác nhau nên hai thông báo không đè nhau.
- Thiếu tài khoản: trả lỗi để retry rồi dead-letter. Lỗi DB: trả lỗi. Record thiếu `Savings` hoặc `Settlement`: bỏ qua.

## 3. Template

Category `SAVINGS`, Severity `SUCCESS`. Không có số dư, số tài khoản đầy đủ hay mã sổ.

| Type | Title | Body | `data` |
|------|-------|------|--------|
| `SAVINGS_OPENED` | Gửi tiết kiệm thành công | `-{amount} VND từ tài khoản •••• {last4} gửi tiết kiệm kỳ hạn {termMonths} tháng.` | `savingsId`, `amount`, `currency`, `termMonths` |
| `SAVINGS_MATURED` | Tất toán tiết kiệm thành công | `+{amount} VND (gồm lãi {interest} VND) vào tài khoản •••• {last4}.` | `savingsId`, `amount`, `currency` |

`amount` của `SAVINGS_MATURED` là tổng nhận về (gốc + lãi).

## 4. Mobile

Cả hai type mở danh sách tiết kiệm (`routes.savings`) vì app chưa có màn chi tiết từng sổ. Khi có màn đó, đổi route theo `data.savingsId`.

## 5. Kiểm thử

| Mức | Nội dung |
|-----|----------|
| Unit mapper + template | Câu chữ, `data`, không lộ số tài khoản đầy đủ, recipient, `SourceEventID`, hai thông báo của một sổ khác ID, record thiếu dữ liệu bị bỏ qua, thiếu tài khoản/lỗi DB trả lỗi |
| Vitest mobile | Hai type mở danh sách tiết kiệm |
| Thử tay | Mở một sổ tiết kiệm và tất toán khi đáo hạn với `make start-notifier` |

## 6. Câu hỏi mở

1. Có muốn nhắc trước khi sổ đáo hạn (ví dụ T-3) không? Cần job quét như `loan-reminder`.
2. Thêm màn chi tiết sổ trên mobile để điều hướng theo `savingsId`?
