# API Spec — Chuyển tiền & Lịch sử giao dịch

---

## 1. Tổng quan

| Thông tin | Chi tiết |
|-----------|----------|
| **Tính năng** | Chuyển tiền nội bộ AnomalyBank và liên ngân hàng (xác thực OTP), lịch sử giao dịch, chi tiết giao dịch |
| **Hiện trạng client** | UI đã xong, đang chạy bằng mock (`client/src/features/transfer/api/transfer.ts`, `client/src/features/transactions/mocks/transactions.ts`) |
| **Mục đích tài liệu** | Liệt kê các API client cần, kèm request/response mẫu, để BE thiết kế và implement |
| **Hiện trạng server** | Chưa có API nào dưới đây. Domain đã có `Withdraw`, `ErrInvalidAmount`, `ErrInsufficientFunds`; OTP hiện có (`/api/auth/otp/*`) chỉ dùng cho đăng ký theo email |
| **Trạng thái** | Draft — chờ BE review |

### 1.1 Màn hình phía client

| Route | Màn hình | API dùng |
|-------|----------|----------|
| `#/transfer` (1 màn hình) | Chọn ngân hàng, nhập STK (tự tra tên), người nhận gần đây, số tiền, lời nhắn | 2.0, 2.1, 2.2, `GET /api/auth/me` (số dư — đã có) |
| `#/transfer` — sheet trượt lên | Tóm tắt giao dịch + nhập OTP | 2.3, 2.4, 2.5 |
| `#/transfer` — sheet kết quả | Biên lai thành công / thất bại | Response của 2.4 |
| `#/transactions` | Lịch sử: tổng thu/chi tháng, tìm kiếm, lọc vào/ra, nhóm theo ngày | 2.6, 2.7 |
| `#/transactions/:id` | Chi tiết giao dịch | 2.8 |

### 1.2 Quy ước chung

- Mọi API bên dưới **yêu cầu** `Authorization: Bearer <jwt>` (middleware `RequireAuth`). Tài khoản nguồn luôn lấy từ `userID` trong JWT, client **không** gửi tài khoản nguồn.
- Số tiền: số nguyên `int64`, đơn vị VND (không có phần thập phân).
- Thời gian: chuỗi RFC 3339 có timezone, ví dụ `"2026-09-22T08:30:00+07:00"`.
- Lỗi: giữ format hiện có `{ "error": "<message>" }`, **đề xuất thêm** trường `code` để client map sang thông báo tiếng Việt thay vì phụ thuộc vào message:

```json
{ "error": "insufficient funds", "code": "INSUFFICIENT_FUNDS" }
```

---

## 2. Danh sách API

| # | Method | Path | Mục đích |
|---|--------|------|----------|
| 2.0 | GET | `https://api.vietqr.io/v2/banks` (bên thứ 3) | Danh sách ngân hàng — **không cần BE làm** |
| 2.1 | GET | `/api/accounts/lookup?bankCode=&accountNo=` | Tra tên chủ tài khoản người nhận |
| 2.2 | GET | `/api/transfers/recent-recipients` | Người nhận gần đây |
| 2.3 | POST | `/api/transfers` | Khởi tạo lệnh chuyển + gửi OTP |
| 2.4 | POST | `/api/transfers/{id}/confirm` | Xác thực OTP và thực hiện chuyển |
| 2.5 | POST | `/api/transfers/{id}/otp/resend` | Gửi lại OTP |
| 2.6 | GET | `/api/transactions` | Danh sách giao dịch (lọc, tìm kiếm, phân trang) |
| 2.7 | GET | `/api/transactions/summary?month=` | Tổng tiền vào/ra trong tháng |
| 2.8 | GET | `/api/transactions/{id}` | Chi tiết một giao dịch |

---

### 2.0 Danh sách ngân hàng (bên thứ ba — BE không cần làm)

Client gọi thẳng `GET https://api.vietqr.io/v2/banks` (CORS mở), chỉ lấy ngân hàng có
`transferSupported = 1`, và tự chèn AnomalyBank lên đầu với `bankCode = "ANOMALY"`.
Các trường client dùng: `code`, `bin`, `shortName`, `name`, `logo`.

BE chỉ cần **chấp nhận `bankCode` theo mã `code` của VietQR** (vd `VCB`, `TCB`, `MB`, `BIDV`)
hoặc `ANOMALY` cho chuyển nội bộ ở các API 2.1–2.3.

---

### 2.1 Tra cứu tài khoản người nhận

`GET /api/accounts/lookup?bankCode=VCB&accountNo=0011001234567`

| Query | Mô tả |
|-------|-------|
| `bankCode` | `ANOMALY` hoặc mã VietQR (`code`) |
| `accountNo` | 6–19 chữ số |

Client tự gọi sau khi người dùng ngừng gõ 500 ms, hoặc khi rời ô số tài khoản / đổi ngân hàng.

**Response — 200**
```json
{
  "accountNo": "0011001234567",
  "name": "NGUYEN VAN AN",
  "bankCode": "VCB"
}
```

`name` viết hoa, không dấu (giống tên trên thẻ). Không trả `id`, email hay bất kỳ thông tin nào khác của người nhận.

**Error responses**

| HTTP | code | Khi nào | Client hiển thị |
|------|------|---------|-----------------|
| 400 | `INVALID_ACCOUNT_NO` | Không phải 6–19 chữ số | "Số tài khoản gồm 6–19 chữ số." |
| 400 | `UNSUPPORTED_BANK` | `bankCode` không hợp lệ / chưa hỗ trợ | "Ngân hàng chưa được hỗ trợ." |
| 404 | `ACCOUNT_NOT_FOUND` | Không tồn tại | "Không tìm thấy tài khoản tại <tên NH>." |
| 422 | `SELF_TRANSFER` | Trùng tài khoản của chính user (chỉ với `ANOMALY`) | "Không thể chuyển tiền đến chính tài khoản của bạn." |
| 502 | `LOOKUP_UNAVAILABLE` | Hệ thống liên ngân hàng lỗi | "Không tra cứu được tài khoản. Vui lòng thử lại." |

---

### 2.2 Người nhận gần đây

`GET /api/transfers/recent-recipients?limit=4`

**Response — 200**
```json
{
  "items": [
    {
      "accountNo": "99999180147",
      "name": "TRAN THI BICH",
      "bankCode": "ANOMALY",
      "lastTransferredAt": "2026-09-21T17:45:00+07:00"
    },
    {
      "accountNo": "0011001234567",
      "name": "NGUYEN VAN AN",
      "bankCode": "VCB",
      "lastTransferredAt": "2026-09-20T09:12:00+07:00"
    }
  ]
}
```

- Lấy từ các giao dịch chuyển **đi thành công** của user, không trùng cặp (`bankCode`, `accountNo`), sắp xếp mới nhất trước.
- Client tự map `bankCode` sang tên + logo từ danh sách 2.0.
- `limit` mặc định 4, tối đa 10. Chưa có giao dịch nào thì trả `items: []`.

---

### 2.3 Khởi tạo lệnh chuyển tiền

`POST /api/transfers`

**Headers**
- `Idempotency-Key: <uuid v4>` — client sinh mới mỗi lần bấm "Xác nhận chuyển tiền", dùng lại khi retry do mất mạng.

**Request**
```json
{
  "toBankCode": "VCB",
  "toAccountNo": "0011001234567",
  "amount": 500000,
  "note": "PHAM MINH HIEU chuyen tien"
}
```

**Validate (server bắt buộc kiểm tra lại, không tin client)**

| Trường | Quy tắc |
|--------|---------|
| `toBankCode` | `ANOMALY` hoặc mã VietQR được hỗ trợ |
| `toAccountNo` | 6–19 chữ số, tồn tại tại `toBankCode`, khác tài khoản nguồn (khi nội bộ) |
| `amount` | ≥ 1.000, ≤ số dư hiện tại (phí nội bộ = 0) |
| `note` | 0–100 ký tự, có thể rỗng |

Server tạo lệnh ở trạng thái `awaiting_otp`, **chưa trừ tiền**, và gửi OTP 6 số qua email đăng ký.

**Response — 201**
```json
{
  "transferId": "trf_01J8ZK3Q7X",
  "status": "awaiting_otp",
  "recipient": {
    "accountNo": "0011001234567",
    "name": "NGUYEN VAN AN",
    "bankCode": "VCB"
  },
  "amount": 500000,
  "fee": 0,
  "note": "PHAM MINH HIEU chuyen tien",
  "otp": {
    "channel": "email",
    "maskedDestination": "ph***@gmail.com",
    "expiresAt": "2026-09-22T08:35:00+07:00",
    "attemptsLeft": 3,
    "resendAvailableAt": "2026-09-22T08:31:00+07:00"
  }
}
```

**Error responses**

| HTTP | code | Client hiển thị |
|------|------|-----------------|
| 400 | `INVALID_AMOUNT` | "Số tiền tối thiểu là 1.000₫." |
| 400 | `INSUFFICIENT_FUNDS` | "Số dư không đủ để thực hiện giao dịch." |
| 400 | `INVALID_NOTE` | "Nội dung tối đa 100 ký tự." |
| 404 | `ACCOUNT_NOT_FOUND` | Như 2.1 |
| 422 | `SELF_TRANSFER` | Như 2.1 |
| 409 | `IDEMPOTENCY_CONFLICT` | Cùng key nhưng khác body → báo lỗi chung |

---

### 2.4 Xác thực OTP và thực hiện chuyển

`POST /api/transfers/{transferId}/confirm`

**Request**
```json
{ "otp": "123456" }
```

Khi OTP đúng: trừ tiền nguồn, cộng tiền đích (ghi event cho **cả hai** tài khoản), sinh bản ghi giao dịch cho cả hai phía.

**Response — 200** — trả về bản ghi giao dịch phía người gửi (schema `Transaction`, xem mục 3):
```json
{
  "id": "tx_01J8ZK4A2M",
  "reference": "FT26265118302",
  "direction": "out",
  "status": "success",
  "kind": "transfer",
  "amount": 500000,
  "fee": 0,
  "note": "PHAM MINH HIEU chuyen tien",
  "counterparty": {
    "name": "TRAN THI BICH",
    "accountNo": "99999180147",
    "bank": "AnomalyBank"
  },
  "createdAt": "2026-09-22T08:31:12+07:00",
  "balanceAfter": 2500000
}
```

**Error responses**

| HTTP | code | Body bổ sung | Client xử lý |
|------|------|--------------|--------------|
| 400 | `INVALID_OTP` | `"attemptsLeft": 2` | Lỗi ngay dưới ô OTP: "Mã OTP không đúng. Bạn còn 2 lần thử." |
| 423 | `OTP_ATTEMPTS_EXCEEDED` | — | Lệnh bị huỷ → màn thất bại "Bạn đã nhập sai OTP 3 lần. Giao dịch đã bị huỷ." |
| 410 | `OTP_EXPIRED` | — | Màn thất bại, cho tạo lệnh mới |
| 400 | `INSUFFICIENT_FUNDS` | — | Số dư đã thay đổi giữa lúc tạo lệnh và xác nhận |
| 404 | `TRANSFER_NOT_FOUND` | — | Lệnh không tồn tại hoặc không thuộc user |
| 409 | `TRANSFER_ALREADY_PROCESSED` | `"transaction": { ... }` | Gọi confirm 2 lần → trả lại kết quả cũ, **không** trừ tiền lần 2 |

Ví dụ lỗi sai OTP:
```json
{ "error": "invalid otp", "code": "INVALID_OTP", "attemptsLeft": 2 }
```

---

### 2.5 Gửi lại OTP

`POST /api/transfers/{transferId}/otp/resend` — không có body.

**Response — 200**
```json
{
  "maskedDestination": "ph***@gmail.com",
  "expiresAt": "2026-09-22T08:40:00+07:00",
  "attemptsLeft": 3,
  "resendAvailableAt": "2026-09-22T08:36:00+07:00"
}
```

**Error responses**
- `429 OTP_RESEND_TOO_SOON` — chưa tới `resendAvailableAt`.
- `404 TRANSFER_NOT_FOUND`, `409 TRANSFER_ALREADY_PROCESSED`.

> Client hiện **chưa** có nút gửi lại OTP; API này để làm ngay sau khi BE có.

---

### 2.6 Danh sách giao dịch

`GET /api/transactions?direction=all&q=tra%20tien&limit=20&cursor=`

| Query | Kiểu | Mặc định | Mô tả |
|-------|------|----------|-------|
| `direction` | `all` \| `in` \| `out` | `all` | Chip lọc Tất cả / Tiền vào / Tiền ra |
| `q` | string | — | Tìm theo tên đối tác, STK, nội dung, mã giao dịch, số tiền. **Không phân biệt hoa thường và dấu** ("tra tien an" khớp "Trả tiền ăn") |
| `from`, `to` | RFC 3339 | — | Lọc khoảng thời gian (dự phòng, UI chưa dùng) |
| `limit` | int | 20 | Tối đa 50 |
| `cursor` | string | — | Lấy từ `nextCursor` của trang trước |

**Response — 200**
```json
{
  "items": [
    {
      "id": "tx_1003",
      "reference": "FT26264175520",
      "direction": "in",
      "status": "success",
      "kind": "transfer",
      "amount": 2500000,
      "fee": 0,
      "note": "Tra tien an trua",
      "counterparty": {
        "name": "TRAN THI BICH",
        "accountNo": "99999180147",
        "bank": "AnomalyBank"
      },
      "createdAt": "2026-09-21T17:45:00+07:00",
      "balanceAfter": 129795000
    }
  ],
  "nextCursor": "eyJ0IjoiMjAyNi0wOS0yMVQxNzo0NTowMFoiLCJpZCI6InR4XzEwMDMifQ"
}
```

- Sắp xếp `createdAt` giảm dần. Hết dữ liệu thì `nextCursor: null`.
- Client tự nhóm theo ngày ("Hôm nay", "Hôm qua", …) nên server **không** cần nhóm.
- Bao gồm cả giao dịch `pending` và `failed` (UI có badge riêng).

---

### 2.7 Tổng thu/chi theo tháng

`GET /api/transactions/summary?month=2026-09`

**Response — 200**
```json
{
  "month": "2026-09",
  "totalIn": 2500000,
  "totalOut": 55000
}
```

- Chỉ tính giao dịch `status = "success"` (không tính `pending`, `failed`).
- Tính theo giờ Việt Nam (UTC+7).
- `month` sai định dạng → `400 INVALID_MONTH`.

> Lý do tách API riêng: danh sách có phân trang nên client không tự cộng được toàn bộ tháng.

---

### 2.8 Chi tiết giao dịch

`GET /api/transactions/{id}`

**Response — 200**: một object schema `Transaction` (giống phần tử trong 2.6).

**Error responses**
- `404 TRANSACTION_NOT_FOUND` — không tồn tại **hoặc không thuộc user** (không trả 403 để tránh lộ id của người khác).

---

## 3. Schema `Transaction`

Khớp với type `TransactionRecord` ở `client/src/features/transactions/model/types.ts`.

| Trường | Kiểu | Bắt buộc | Mô tả |
|--------|------|----------|-------|
| `id` | string | ✔ | ID nội bộ, dùng cho route `#/transactions/:id` |
| `reference` | string | ✔ | Mã giao dịch cho người dùng (tra soát), ví dụ `FT26265118302` |
| `direction` | `"in"` \| `"out"` | ✔ | Góc nhìn của user đang đăng nhập |
| `status` | `"success"` \| `"pending"` \| `"failed"` | ✔ | |
| `kind` | `"transfer"` \| `"payment"` \| `"bill"` \| `"topup"` \| `"savings"` \| `"salary"` | ✔ | MVP chỉ cần `transfer`; các loại khác để dành cho tính năng sau |
| `amount` | int64 | ✔ | Luôn dương; dấu +/- do `direction` quyết định |
| `fee` | int64 | ✔ | `0` với chuyển nội bộ |
| `note` | string | ✔ | Có thể rỗng |
| `counterparty.name` | string | ✔ | Người nhận (nếu `out`) hoặc người gửi (nếu `in`) |
| `counterparty.accountNo` | string | — | Có với `transfer` |
| `counterparty.bank` | string | — | Có với `transfer` — tên viết tắt ngân hàng (vd `Vietcombank`) |
| `counterparty.bankCode` | string | — | Có với `transfer` — `ANOMALY` hoặc mã VietQR, để client hiện logo |
| `createdAt` | RFC 3339 | ✔ | |
| `balanceAfter` | int64 \| null | — | Số dư sau giao dịch; `null` nếu `pending`/`failed` |

Một lệnh chuyển thành công sinh **2 bản ghi**: người gửi thấy `direction: "out"`, người nhận thấy `direction: "in"`, cùng `reference`.

---

## 4. Quy tắc nghiệp vụ (client đang áp dụng, server cần enforce)

| Quy tắc | Giá trị |
|---------|---------|
| Ngân hàng | AnomalyBank (nội bộ) + ngân hàng VietQR có `transferSupported = 1` |
| Số tài khoản | 6–19 chữ số |
| Số tiền tối thiểu | 1.000₫ |
| Số tiền tối đa | ≤ số dư khả dụng (hạn mức/ngày: xem OQ-02) |
| Phí | 0₫ (liên ngân hàng: xem OQ-08) |
| Nội dung | ≤ 100 ký tự; client điền sẵn `"<TÊN KHÔNG DẤU> chuyen tien"` |
| OTP | 6 chữ số, gửi email, tối đa 3 lần nhập sai thì huỷ lệnh |
| Tự chuyển cho mình | Không cho phép |
| Chuyển trùng | Chặn bằng `Idempotency-Key` (2.3) và trạng thái lệnh (2.4) |

Sau khi chuyển thành công, `GET /api/auth/me` phải trả `amount` mới (client đang lấy số dư từ đây). Nếu projection trễ, xem OQ-05.

---

## 5. Mapping client ↔ API (để thay mock)

| Mock hiện tại | Thay bằng |
|---------------|-----------|
| `lookupRecipient(bank, accountNo)` | 2.1 |
| `fetchBanks()` (VietQR) | Giữ nguyên — không cần BE |
| `recentRecipients` (hằng số) | 2.2 |
| `submitTransfer({...otp})` (gộp 1 bước) | Tách thành 2.3 (khi bấm "Xác nhận chuyển tiền") + 2.4 (khi nhập OTP) |
| `DEMO_OTP = "123456"` | Xoá, bỏ dòng gợi ý OTP demo ở `OtpStep` |
| `transactionStore.list()` | 2.6 |
| Tổng tháng tính ở client (`TransactionHistory`) | 2.7 |
| `transactionStore.get(id)` | 2.8 |

---

## 6. Open Questions cho BE

| # | Câu hỏi | Ảnh hưởng |
|---|---------|-----------|
| OQ-01 | Dùng lại hạ tầng OTP email + Redis hiện có (`/api/auth/otp/*`) hay tách service OTP riêng cho giao dịch? | Thiết kế 2.3–2.5 |
| OQ-02 | Có hạn mức mỗi giao dịch / mỗi ngày không? Nếu có, thêm code `LIMIT_EXCEEDED` | Validate 2.3 |
| OQ-03 | TTL của OTP và lệnh `awaiting_otp` bao lâu? (client đề xuất 5 phút) | 2.3, 2.4 |
| OQ-04 | Chuyển tiền giữa 2 stream `account-{id}` trong KurrentDB có cần saga/transaction không, hay một event `TransferCompleted` dùng chung? | Tính nhất quán số dư |
| OQ-05 | Projection Mongo trễ bao lâu sau khi chuyển? Có nên trả `balanceAfter` ở 2.4 để client cập nhật ngay mà không phải chờ `/me`? | UX số dư trên Dashboard |
| OQ-06 | Có trạng thái `pending` thật không (chuyển nội bộ thường xong ngay)? | Badge "Đang xử lý" trên UI |
| OQ-07 | Có đồng ý thêm trường `code` vào error response chung không? | Toàn bộ mục lỗi |
| OQ-08 | Chuyển liên ngân hàng có thu phí không? Nếu có, trả `fee` ở 2.3 để client hiện trong sheet xác nhận | UI sheet xác nhận |
| OQ-09 | Tra cứu/chuyển liên ngân hàng đi qua cổng nào (Napas 247…)? Nếu chưa có, BE mock theo `bankCode` như client đang làm | Phạm vi BE |

---

*Phiên bản: 0.2 — thêm chọn ngân hàng (liên ngân hàng)*
*Ngày tạo: 2026-09-22*
*Trạng thái: Draft*
