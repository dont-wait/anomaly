# API Spec — Trả nợ vay

---

## 1. Tổng quan

| Thông tin | Chi tiết |
|-----------|----------|
| **Tính năng** | Khách hàng xem lịch trả nợ, số tiền đến hạn, thực hiện trả nợ trên app, tra cứu các lần thanh toán; hệ thống có thể thu nợ tự động theo lịch |
| **Nguồn nghiệp vụ** | `QuyTrinhHeThong.docx` (UC-S08), `QuyTrinhNghiepVu.docx` (UC-05), chính sách lãi suất V2 |
| **Quan hệ tài liệu** | Sử dụng khoản vay được tạo từ `loan-application-spec.md`; **không** lặp API tạo hồ sơ/giải ngân |
| **Mục đích** | Chốt API client ↔ BE trước khi lập trình thanh toán khoản vay |
| **Hiện trạng code** | Chưa xác minh trong repo, tất cả endpoint/màn hình bên dưới là **đề xuất** |
| **Trạng thái** | Draft v0.1 — chờ BE review |

### 1.1 Màn hình phía client (đề xuất)

| Route dự kiến | Màn hình | API dùng |
|---------------|----------|----------|
| `#/loans/:id` | Thông tin dư nợ, kỳ sắp đến hạn | `GET /api/loans/{id}` và 2.1 |
| `#/loans/:id/schedule` | Lịch gốc, lãi, tổng phải trả từng kỳ | 2.1 |
| `#/loans/:id/repay` | Chọn tài khoản thanh toán, xem số tiền đến hạn | 2.2 |
| `#/loans/:id/repay` — xác nhận | Xem lại tổng tiền, xác nhận thanh toán | 2.3 |
| `#/loans/:id/payments` | Danh sách lần trả và chứng từ | 2.4, 2.5 |

### 1.2 Quy ước chung

- `Authorization: Bearer <jwt>`; BE đối chiếu khoản vay và tài khoản thanh toán với khách hàng đang đăng nhập.
- Tiền là `int64` VND; ngày/giờ RFC 3339. Lãi suất `%/năm` được lấy từ **hợp đồng vay đã ký**, không lấy lại biểu lãi hiện hành.
- Phần mềm chỉ hiển thị khoản tiền **server tính**, không nhận số tiền gốc/lãi do client tự sửa.
- POST tạo giao dịch cần header `Idempotency-Key: <uuid v4>`; retry không trừ tiền hai lần.
- Lỗi trả `{ "error": "<message>", "code": "<ERROR_CODE>" }`.
- Việc **thu nợ tự động** nếu có do job/scheduler BE kích hoạt, không dùng timer từ client.

---

## 2. Danh sách API đề xuất

| # | Method | Path | Mục đích |
|---|--------|------|----------|
| 2.1 | GET | `/api/loans/{id}/repayment-schedule` | Xem lịch trả nợ và kỳ hiện tại |
| 2.2 | POST | `/api/loans/{id}/repayments/quote` | Tính số tiền cần trả ở thời điểm hiện tại |
| 2.3 | POST | `/api/loans/{id}/repayments` | Thực hiện thanh toán kỳ vay |
| 2.4 | GET | `/api/loans/{id}/payments` | Lịch sử những lần trả nợ |
| 2.5 | GET | `/api/loans/{id}/payments/{paymentId}` | Chi tiết kết quả trả nợ |

### 2.1 Lịch trả nợ

`GET /api/loans/loan_001/repayment-schedule`

**Response — 200** (con số minh họa, phải được tính từ lịch trả nợ thực)

```json
{
  "loanId": "loan_001",
  "loanNo": "LN20261014001",
  "principal": 50000000,
  "outstandingPrincipal": 50000000,
  "annualInterestRate": "16.3",
  "termMonths": 12,
  "repaymentMethod": "fixed_total_reducing_balance",
  "items": [
    {
      "installmentNo": 1,
      "dueDate": "2026-11-14",
      "principalDue": 3864477,
      "interestDue": 679167,
      "totalDue": 4543644,
      "status": "upcoming"
    }
  ],
  "nextInstallmentNo": 1
}
```

**Lưu ý số liệu:** Với ví dụ 50 triệu, 16,3%/năm, 12 tháng, số tiền kỳ đầu trên được làm tròn theo **giả định tạm tính** lãi tháng = lãi năm/12, gốc giảm dần và tổng trả đều. Công thức thực tế, cách làm tròn và xử lý kỳ cuối là **OQ-01**, không dùng số mẫu làm giá trị cố định.

- BE trả đủ các kỳ, có thể bổ sung phân trang nếu kỳ hạn dài.
- `status` dự kiến: `upcoming`, `due`, `paid`, `overdue` (cần chốt enum với DB).
- Nếu số dư nợ bằng 0, trạng thái khoản vay phải chuyển `completed`/`closed` nhất quán với schema thực tế.

**Error responses**

| HTTP | code | Khi nào |
|------|------|---------|
| 404 | `LOAN_NOT_FOUND` | Khoản vay không tồn tại hoặc không thuộc user |
| 409 | `LOAN_NOT_DISBURSED` | Hồ sơ chưa thành khoản vay đã giải ngân |

### 2.2 Xem trước số tiền thanh toán

`POST /api/loans/loan_001/repayments/quote`

**Request**

```json
{
  "installmentNo": 1,
  "sourceAccountId": "acc_001"
}
```

**Response — 200**

```json
{
  "quoteId": "repay_q_001",
  "loanId": "loan_001",
  "installmentNo": 1,
  "dueDate": "2026-11-14",
  "principalAmount": 3864477,
  "interestAmount": 679167,
  "penaltyAmount": 0,
  "totalAmount": 4543644,
  "currency": "VND",
  "sourceAccount": {
    "accountId": "acc_001",
    "availableBalance": 6000000
  },
  "quoteExpiresAt": "2026-11-14T10:05:00+07:00"
}
```

- Số tiền là **server tính tại thời điểm quote**, bao gồm gốc/lãi và khoản phạt nếu có chính sách. `penaltyAmount: 0` chỉ minh họa kỳ chưa quá hạn.
- Nguồn tiền phải thuộc khách hàng, còn hoạt động và đủ số dư khi **thực hiện thực tế**, không chỉ khi xem quote.
- Chưa cho client tự nhập `totalAmount` khác số tiền kỳ hạn cho tới khi nhóm quyết định chính sách trả một phần/trả trước hạn.

**Error responses**

| HTTP | code | Client hiển thị |
|------|------|-----------------|
| 404 | `LOAN_NOT_FOUND` | "Không tìm thấy khoản vay." |
| 404 | `INSTALLMENT_NOT_FOUND` | "Không tìm thấy kỳ cần thanh toán." |
| 409 | `INSTALLMENT_ALREADY_PAID` | "Kỳ này đã thanh toán." |
| 422 | `INVALID_PAYMENT_ACCOUNT` | "Tài khoản thanh toán không hợp lệ." |
| 422 | `PAYMENT_POLICY_NOT_SUPPORTED` | "Hình thức thanh toán hiện chưa hỗ trợ." |

### 2.3 Thực hiện thanh toán

`POST /api/loans/loan_001/repayments`

**Headers:** `Idempotency-Key: <uuid v4>`

**Request**

```json
{
  "quoteId": "repay_q_001",
  "sourceAccountId": "acc_001"
}
```

**Response — 200**

```json
{
  "paymentId": "loan_pay_001",
  "loanId": "loan_001",
  "transactionId": "tx_loan_repay_001",
  "installmentNo": 1,
  "status": "success",
  "amount": 4543644,
  "principalAmount": 3864477,
  "interestAmount": 679167,
  "penaltyAmount": 0,
  "outstandingPrincipalAfter": 46135523,
  "paidAt": "2026-11-14T10:01:00+07:00"
}
```

**Xử lý bắt buộc ở BE:**

1. Xác thực user, quyền sở hữu khoản vay và tài khoản thanh toán.
2. Kiểm tra quote còn hiệu lực, trạng thái kỳ vay và số dư hiện tại; **tính/kiểm tra lại tiền phải thu** trước khi ghi sổ.
3. Ghi nhận việc ghi nợ tài khoản nguồn và thanh toán kỳ vay một cách an toàn, tránh trừ tiền thành công nhưng `loan_payments` thất bại gây mất cân đối.
4. Cập nhật số dư gốc còn lại và kỳ trả nợ, lưu transaction/ledger, event và audit.
5. Với request trùng `Idempotency-Key`, trả kết quả cũ hoặc trạng thái đã xử lý, **không thu tiền lần hai**.

**Error responses**

| HTTP | code | Khi nào | Client xử lý |
|------|------|---------|--------------|
| 400 | `INSUFFICIENT_FUNDS` | Không đủ số dư | Thông báo, cho chọn tài khoản khác |
| 404 | `LOAN_NOT_FOUND` | Không tồn tại/không thuộc user | Về danh sách |
| 409 | `INSTALLMENT_ALREADY_PAID` | Đã thu kỳ này | Hiển thị kết quả đã có, không gọi lại |
| 409 | `PAYMENT_ALREADY_PROCESSED` | Retry cùng lệnh | Trả kết quả cũ |
| 409 | `IDEMPOTENCY_CONFLICT` | Cùng key, body khác | Thông báo lỗi |
| 410 | `QUOTE_EXPIRED` | Quote không còn hợp lệ | Gọi lại 2.2 |
| 422 | `LOAN_NOT_ACTIVE` | Khoản vay chưa hoạt động/đã tất toán | Không cho thanh toán |
| 503 | `PAYMENT_UNAVAILABLE` | Hệ thống tạm không xử lý được | Kiểm tra trạng thái trước khi retry |

> **OTP thanh toán:** Tài liệu nghiệp vụ chưa chốt OTP cho trả nợ thủ công. Nếu quyết định áp dụng OTP, cần bổ sung luồng khởi tạo/chấp nhận riêng, **không tự lược bỏ bước bảo mật khi triển khai**.

### 2.4 Lịch sử thanh toán khoản vay

`GET /api/loans/loan_001/payments?limit=20&cursor=`

**Response — 200**

```json
{
  "items": [
    {
      "paymentId": "loan_pay_001",
      "installmentNo": 1,
      "amount": 4543644,
      "principalAmount": 3864477,
      "interestAmount": 679167,
      "penaltyAmount": 0,
      "status": "success",
      "paidAt": "2026-11-14T10:01:00+07:00"
    }
  ],
  "nextCursor": null
}
```

- Chỉ các kỳ thuộc khoản vay khách hàng được xem; sắp xếp mới nhất trước.
- Các lần trả lỗi/đang xử lý cần trạng thái rõ ràng, không coi là đã thanh toán.

### 2.5 Chi tiết thanh toán

`GET /api/loans/loan_001/payments/loan_pay_001`

**Response — 200**

```json
{
  "paymentId": "loan_pay_001",
  "transactionId": "tx_loan_repay_001",
  "loanId": "loan_001",
  "installmentNo": 1,
  "channel": "bank_account",
  "amount": 4543644,
  "principalAmount": 3864477,
  "interestAmount": 679167,
  "penaltyAmount": 0,
  "status": "success",
  "paidAt": "2026-11-14T10:01:00+07:00"
}
```

Lỗi: `404 PAYMENT_NOT_FOUND` khi không tồn tại hoặc không thuộc khoản vay của khách hàng.

---

## 3. Schema và liên hệ MongoDB

| API field | Collection/trường hiện có | Ghi chú |
|-----------|---------------------------|---------|
| `loanId`, `outstandingPrincipal` | `loans._id`, `outstanding_principal` | Trạng thái dư nợ |
| `annualInterestRate` | `loans.interest_rate` | Mức đã ký, không lấy biểu mới |
| `installmentNo`, `dueDate`, `principalDue`, `interestDue`, `totalDue` | `loans.repayment_schedule[]` | Lịch gốc/lãi của các kỳ |
| `paymentId`, `principalAmount`, `interestAmount`, `penaltyAmount` | `loan_payments` | Các lần thực trả |
| `transactionId` | `loan_payments.transaction_id` → `transactions` | Liên kết giao dịch tiền |
| Số dư ghi nợ | `accounts`, `ledger_entries` | Ghi nhận dòng tiền có thể truy vết |
| Event nghiệp vụ | Event store / `outbox_events` | Ví dụ `LoanInstallmentPaid` |

Schema hiện có chưa định nghĩa `quoteId` bền vững; đây là token xác nhận đề xuất, có thể lưu ngắn hạn trong Redis nếu nhóm chọn cơ chế quote/confirm.

---

## 4. Quy tắc nghiệp vụ

1. Sau giải ngân, ngân hàng đã lập lịch trả nợ từng tháng. Theo tài liệu nghiệp vụ: **tổng trả hằng tháng cố định**, lãi tính trên dư nợ gốc giảm dần, gốc trả tăng dần.
2. Lãi suất tính từ `loans.interest_rate` đã ký. Thay đổi lãi suất của gói vay **không** sửa lịch cũ nếu hợp đồng cố định.
3. Chỉ ghi `paid` khi tiền thực sự được hạch toán thành công; phải có `transactionId` và log/event tương ứng.
4. Không thu trùng một kỳ khi request bị retry, app mất mạng hoặc user nhấn nhiều lần.
5. Với nợ quá hạn: ghi trạng thái quá hạn; **chưa tự đặt tỷ lệ phạt và cách tính nợ quá hạn**.
6. Với số dư không đủ: không trừ tiền một phần nếu không có chính sách rõ ràng; thông báo và giữ kỳ chưa thanh toán.
7. Việc thu nợ tự động theo ngày đến hạn phải kiểm tra lại số dư, chống chạy job trùng và có kết quả `success`/`failed`; thất bại thì thông báo. **Chưa bật mặc định** khi chưa chốt quyền tự trích nợ.
8. Event gợi ý: `LoanRepaymentDue`, `LoanRepaymentSucceeded`, `LoanRepaymentFailed`, `LoanBecameOverdue`, `LoanFullyRepaid`. Đây chỉ là tên đề xuất để BE thống nhất event naming.

---

## 5. Mapping client ↔ API

| Hành động | API |
|-----------|-----|
| Xem khoản vay / số dư nợ | `GET /api/loans/{id}` (định nghĩa trong loan spec) |
| Xem lịch trả | `GET /api/loans/{id}/repayment-schedule` |
| Chọn kỳ và tài khoản trả | `POST /api/loans/{id}/repayments/quote` |
| Xác nhận thanh toán | `POST /api/loans/{id}/repayments` |
| Xem giao dịch trả nợ | `GET /api/loans/{id}/payments` |
| Xem biên nhận | `GET /api/loans/{id}/payments/{paymentId}` |

---

## 6. Open Questions cho nhóm / BE

| # | Câu hỏi | Ảnh hưởng |
|---|---------|-----------|
| OQ-01 | Công thức trả góp đều, ngày tính lãi (tháng/365 ngày), quy tắc làm tròn và xử lý phần dư ở kỳ cuối? | Tất cả khoản tiền và lịch |
| OQ-02 | Bản đầu cho phép trả **đúng số phải trả từng kỳ** hay trả một phần/trả nhiều kỳ/trả trước hạn? | Validation, quote, ledger |
| OQ-03 | Nếu trả trước hạn: phạt bao nhiêu, giảm tổng lãi hay rút ngắn kỳ hạn? | Nghiệp vụ quan trọng chưa chốt |
| OQ-04 | Quá hạn: số ngày ân hạn, lãi quá hạn/phí phạt theo quy định nào? | `penaltyAmount`, `overdue` |
| OQ-05 | Thanh toán thủ công có cần OTP email? Có cho thanh toán từ nguồn ngoài ngân hàng/tiền mặt trong MVP không? | API xác thực và kênh trả |
| OQ-06 | Có cho hệ thống tự trích nợ từ tài khoản liên kết không? Cách người dùng đồng ý? | Scheduler/background job |
| OQ-07 | Nếu thu nợ thành công nhưng read model Mongo chưa kịp cập nhật thì client xem trạng thái nào? | Nhất quán eventual, UX |
| OQ-08 | Hạch toán phần gốc/lãi và giao dịch tiền liên quan dùng event/stream nào, chống double debit ra sao? | Event Sourcing, ledger |
| OQ-09 | Quy ước `pending`/`success`/`failed` và `paid`/`overdue`/`completed` có khớp enum backend? | API/DB |

---

*Phiên bản: 0.1 — đặc tả độc lập cho Trả nợ vay.*  
*Ngày soạn: 2026-10-12.*  
*Trạng thái: Draft — chờ nhóm review, chưa code.*
