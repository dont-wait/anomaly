# Transaction API: mapping từ client xuống server

> **Trạng thái: MVP chuyển khoản nội bộ đã nối API vào client.** Schema tham chiếu tab **Detailed Fields** trong `MongoDB_Banking_Schema(3)(1).drawio`; migration `000004_create_transaction_collections` và routes server đã được thêm. Chuyển khoản liên ngân hàng chưa được hỗ trợ.

## Phạm vi và hiện trạng

- Client có UI chuyển tiền, lịch sử, chi tiết, dashboard; giao dịch, recent recipients và summary hiện lấy từ API.
- Luồng chuyển tạo lệnh pending, gửi OTP, rồi confirm bằng `transferId`; không còn demo OTP.
- Server có transaction routes cho lookup nội bộ, OTP transfer, recent recipients, danh sách/tổng/chi tiết transaction.
- Migration `000004_create_transaction_collections` tạo validator và index cho transaction, ledger, account feed và outbox. Confirm ghi các collection cùng account balance trong MongoDB session transaction.
- Mapping này chỉ bao phủ giao dịch chuyển khoản và màn hình dùng `TransactionRecord`. Các loại `payment`, `bill`, `topup`, `savings`, `salary` hiện là seed demo, không nằm trong phạm vi ghi/đọc nghiệp vụ của API MVP.

Nguồn schema tham chiếu: tab **Detailed Fields** của sơ đồ MongoDB. Thiết kế phân biệt dữ liệu ghi (`transactions`, `ledger_entries`) và projection đọc (`account_transaction_feed`). API lịch sử nên đọc feed; không dựng lịch sử bằng cách quét ledger.

Nguồn client chính:

| Thành phần | Trách nhiệm hiện tại |
|---|---|
| `client/mobile/src/features/transfer/api/transfer.ts` | Tra cứu, tạo lệnh, xác nhận/gửi lại OTP; danh bạ đã lưu còn tạm thời trong RAM |
| `client/mobile/src/features/transfer/useTransferFlow.ts` | Debounce tra cứu, giữ transfer id, xác nhận OTP và cập nhật số dư hiển thị |
| `client/mobile/src/features/transactions/api.ts` | Adapter lịch sử, summary và chi tiết; parse response thành `TransactionRecord` |
| `client/mobile/src/features/transactions/model/types.ts` | Kiểu `TransactionRecord` hiển thị |
| `client/mobile/src/pages/TransactionHistoryPage.tsx` | Gọi API danh sách/tổng tháng, filter/search server-side và tải tiếp bằng cursor |
| `client/mobile/src/pages/TransactionDetailPage.tsx` | Lấy giao dịch theo id từ API |
| `client/mobile/src/pages/DashboardPage.tsx` | Hiện bốn giao dịch mới nhất từ API |

## Quy ước contract

- API cần xác thực bằng `Authorization: Bearer <JWT>`.
- Server xác định chủ tài khoản từ JWT, tìm account thuộc customer đó; client không gửi `userID`, `accountId` hay số tài khoản nguồn làm căn cứ phân quyền. Query feed bắt buộc scope bằng `account_id` server-resolved.
- Tiền là số nguyên VND, luôn dương trong `amount`; `direction` xác định thu/chi.
- Thời gian trả RFC 3339 kèm timezone. Client parse thành `Date`.
- Lỗi API transaction trả `{ "error": "...", "code": "..." }`; auth middleware cũ có thể chỉ trả `{ "error": "..." }`.
- API hiện chỉ nhận `bankCode=ANOMALY`; bank code khác trả `UNSUPPORTED_BANK` cho tới khi có cổng chuyển liên ngân hàng.

## Mapping theo luồng UI

| Hành động/màn hình | Client hiện tại | API được gọi |
|---|---|---|
| Tra cứu người nhận | `lookupRecipient(bank, accountNo)` | `GET /api/accounts/lookup?bankCode=...&accountNo=...` |
| Ngân hàng | `useBanks()` chỉ hiển thị AnomalyBank do server mới hỗ trợ chuyển nội bộ | Không gọi VietQR cho luồng chuyển |
| Người nhận gần đây | `recentRecipients`/`contactStore` mock | `GET /api/transfers/recent-recipients?limit=4` |
| Bắt đầu chuyển, gửi OTP | Tạo request với `Idempotency-Key`; giữ `transferId` | `POST /api/transfers` |
| Xác nhận OTP | Gửi OTP cùng `transferId`; số lần còn lại lấy từ lỗi server | `POST /api/transfers/{transferId}/confirm` |
| Gửi lại OTP | Nút gửi lại gọi server sau cooldown UI | `POST /api/transfers/{transferId}/otp/resend` |
| Dashboard gần đây | `transactionStore.list(accountNo).slice(0, 4)` | Dùng `GET /api/transactions?limit=4` hoặc chung API lịch sử |
| Lịch sử | Cursor pagination; filter/tìm ở server; tổng tháng lấy summary API | `GET /api/transactions` + `GET /api/transactions/summary` |
| Chi tiết | `transactionStore.get(id, accountNo)` | `GET /api/transactions/{id}` |

Danh sách ngân hàng hiện chỉ gồm AnomalyBank vì backend từ chối bank code khác. Client gửi `bankCode=ANOMALY`; khi có cổng liên ngân hàng mới bật lại VietQR và ánh xạ tên/logo tương ứng.

## Contract API đề xuất

### Tra cứu tài khoản nhận — đã triển khai nội bộ

`GET /api/accounts/lookup?bankCode=ANOMALY&accountNo=99999180147`

Phản hồi `200`:

```json
{
  "accountNo": "99999180147",
  "name": "TRAN THI BICH",
  "bankCode": "ANOMALY"
}
```

Validate số tài khoản 6–19 chữ số, bank hỗ trợ và tài khoản tồn tại. Với chuyển nội bộ, từ chối tự chuyển. Không trả email, user id hoặc dữ liệu hồ sơ khác của người nhận.

Lỗi: `400 INVALID_ACCOUNT_NO`, `400 UNSUPPORTED_BANK`, `404 ACCOUNT_NOT_FOUND`, `422 SELF_TRANSFER`. Chuyển liên ngân hàng chưa có adapter nên bank code khác trả `UNSUPPORTED_BANK`.

### Người nhận gần đây — đã triển khai

`GET /api/transfers/recent-recipients?limit=4`

Phản hồi `200`:

```json
{
  "items": [
    {
      "accountNo": "99999180147",
      "name": "TRAN THI BICH",
      "bankCode": "ANOMALY",
      "lastTransferredAt": "2026-09-21T17:45:00+07:00"
    }
  ]
}
```

Chỉ lấy người nhận từ feed giao dịch chuyển đi thành công của user hiện tại; duy nhất theo cặp `bankCode` + `accountNo`, mới nhất trước. `limit` mặc định 4, tối đa 10.

### Tạo lệnh chuyển và gửi OTP — đã triển khai nội bộ

`POST /api/transfers`

Header đề xuất: `Idempotency-Key: <UUID v4>`; cùng key và cùng body trả lại cùng kết quả, cùng key nhưng body khác trả `409 IDEMPOTENCY_CONFLICT`.

Request:

```json
{
  "toBankCode": "ANOMALY",
  "toAccountNo": "99999180147",
  "amount": 500000,
  "note": "PHAM MINH HIEU chuyen tien"
}
```

Server yêu cầu JWT đã xác minh KYC, kiểm tra lại recipient nội bộ, số tiền tối thiểu 1.000 VND, số dư và note tối đa 100 ký tự; phí hiện bằng 0. Tạo lệnh `awaiting_otp`, chưa ghi nhận chuyển tiền hoàn tất/chưa trừ tiền, rồi gửi OTP tới email đăng ký. OTP có TTL 5 phút, tối đa 3 lần thử; gửi lại bị giới hạn tối thiểu 30 giây.

Phản hồi `201`:

```json
{
  "transferId": "trf_01J8ZK3Q7X",
  "status": "awaiting_otp",
  "recipient": {
    "accountNo": "99999180147",
    "name": "TRAN THI BICH",
    "bankCode": "ANOMALY"
  },
  "amount": 500000,
  "fee": 0,
  "note": "PHAM MINH HIEU chuyen tien",
  "otp": {
    "channel": "email",
    "maskedDestination": "ph***@gmail.com",
    "expiresAt": "2026-09-30T08:35:00+07:00",
    "attemptsLeft": 3,
    "resendAvailableAt": "2026-09-30T08:31:00+07:00"
  }
}
```

Lỗi: `400 INVALID_AMOUNT`, `400 INSUFFICIENT_FUNDS`, `400 INVALID_NOTE`, `404 ACCOUNT_NOT_FOUND`, `422 SELF_TRANSFER`, `409 IDEMPOTENCY_CONFLICT`, `429 OTP_RESEND_TOO_SOON`, `503 OTP_DELIVERY_FAILED`. Chuyển khoản yêu cầu JWT có `isVerify=true`.

### Xác nhận OTP và hoàn tất chuyển — đã triển khai nội bộ

`POST /api/transfers/{transferId}/confirm`

Request:

```json
{ "otp": "123456" }
```

Khi OTP hợp lệ, server thực hiện cập nhật tài khoản nguồn/đích theo một ranh giới nhất quán và trả transaction theo góc nhìn người gửi. Account balance + transaction + debit/credit ledger + hai feed entries + outbox được commit trong cùng MongoDB session transaction. OTP chỉ bị consume sau commit để lỗi DB còn retry được. MongoDB phải là replica set hoặc sharded cluster.

Phản hồi `200` (schema `Transaction` bên dưới):

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
    "bankCode": "ANOMALY",
    "bank": "AnomalyBank"
  },
  "createdAt": "2026-09-30T08:31:12+07:00",
  "balanceAfter": 2500000
}
```

Lỗi: `400 INVALID_OTP` (có `attemptsLeft`), `423 OTP_ATTEMPTS_EXCEEDED`, `410 OTP_EXPIRED`, `400 INSUFFICIENT_FUNDS`, `404 TRANSFER_NOT_FOUND`, `409 TRANSFER_ALREADY_PROCESSED`. Gọi confirm lặp lại trả feed cũ và không trừ tiền lần hai.

### Gửi lại OTP — đã triển khai

`POST /api/transfers/{transferId}/otp/resend`, không body. Client hiện chưa có nút này.

Phản hồi `200` gồm `maskedDestination`, `expiresAt`, `attemptsLeft`, `resendAvailableAt`. Lỗi `429 OTP_RESEND_TOO_SOON`, `404 TRANSFER_NOT_FOUND`, `409 TRANSFER_ALREADY_PROCESSED`. Cooldown 30 giây tính theo email tài khoản.

### Danh sách giao dịch — đã triển khai

`GET /api/transactions?direction=all&q=tra%20tien&limit=20&cursor=`

Query:

| Tên | Giá trị | Ý nghĩa |
|---|---|---|
| `direction` | `all` \| `in` \| `out`, mặc định `all` | Filter chip hiện có |
| `q` | string, tùy chọn | Tìm tên/ số tài khoản đối tác, note, reference, amount; không phân biệt hoa thường và dấu như client |
| `from`, `to` | RFC 3339, tùy chọn | Khoảng thời gian; UI hiện chưa hiển thị filter ngày |
| `limit` | số nguyên, mặc định 20, tối đa 50 | Kích thước trang; dashboard có thể yêu cầu 4 |
| `cursor` | opaque string | Cursor từ phản hồi trước |

Phản hồi `200`:

```json
{
  "items": [],
  "nextCursor": null
}
```

Sắp xếp `createdAt` giảm dần, cursor ổn định khi nhiều bản ghi cùng thời điểm. Feed ghi giao dịch thành công và transfer bị hủy do OTP; map `cancelled` thành `failed`. Lệnh `awaiting_otp` chưa xuất hiện trong history feed. Client tiếp tục nhóm theo ngày để hiển thị. Search/filter được xử lý server-side trên `search_text` đã chuẩn hóa bỏ dấu.

### Tổng thu/chi tháng — đã triển khai

`GET /api/transactions/summary?month=2026-09`

Phản hồi `200`:

```json
{
  "month": "2026-09",
  "totalIn": 2500000,
  "totalOut": 55000
}
```

Chỉ tính `success`, theo tháng lịch Việt Nam (UTC+07:00), dùng cùng phạm vi user như danh sách. Tách endpoint để tổng không bị giới hạn bởi cursor/page đang tải. Sai month trả `400 INVALID_MONTH`.

### Chi tiết giao dịch — đã triển khai

`GET /api/transactions/{id}` trả một object `Transaction`. Giao dịch không tồn tại hoặc không thuộc user đều trả `404 TRANSACTION_NOT_FOUND`, tránh làm lộ id của user khác.

## Schema client cần nhận

Ánh xạ tới `TransactionRecord` ở `client/src/features/transactions/model/types.ts`:

| JSON | Type client | Quy tắc |
|---|---|---|
| `id` | `string` | ID dùng trong route `#/transactions/:id` |
| `reference` | `string` | Mã tra soát hiển thị |
| `direction` | `"in" \| "out"` | Góc nhìn của user hiện tại |
| `status` | `"success" \| "pending" \| "failed"` | Trạng thái hiển thị |
| `kind` | `"transfer" \| "payment" \| "bill" \| "topup" \| "savings" \| "salary"` | MVP transaction API chỉ cần `transfer` |
| `amount`, `fee` | số nguyên VND | `amount` dương; `fee` không âm |
| `note` | `string` | Có thể rỗng |
| `counterparty.name` | `string` | Tên phía đối tác |
| `counterparty.accountNo` | `string?` | Có với transfer khi dữ liệu cho phép |
| `counterparty.bank` | `string?` | Tên hiển thị, ví dụ `AnomalyBank` |
| `counterparty.bankCode` | `string?` | Mã để map logo; type client hiện cần bổ sung nếu API dùng field này |
| `createdAt` | RFC 3339 → `Date` | Adapter client phải parse chuỗi JSON |
| `balanceAfter` | `number?` | `null`/vắng mặt nếu không áp dụng; chỉ trả khi server có giá trị đáng tin cậy |

`ownerAccountNo` là mock-only, không trả từ server. Một chuyển nội bộ có thể tạo hai góc nhìn (nguồn `out`, đích `in`) cùng reference; việc lưu hai transaction hay một transfer cộng ledger entries phải đợi migration/domain design. Client chỉ cần nhận đúng một bản ghi thuộc user đang xác thực.

## Mapping theo Detailed Fields (tham chiếu sơ đồ, cần xác minh với migration)

| Client/API | Collection và field trong sơ đồ | Ghi chú |
|---|---|---|
| Account nguồn | `accounts._id`, `accounts.customer_id`, `accounts.account_no`, `accounts.balance.current`, `accounts.version` | Resolve account từ customer trong JWT; `account_no` chỉ là snapshot/hiển thị, không phải khóa phân quyền từ request. |
| Transfer record | `transactions.transaction_id`, `reference`, `type`, `source.account_id`, `source.account_no`, `destination.type`, `destination.account_id`, `destination.account_no`, `destination.bank_code`, `destination.name`, `amount`, `fee`, `currency`, `channel`, `status`, `authentication.*`, `device.*`, `risk.*`, `note`, `idempotency_key`, `created_at`, `posted_at` | Migration áp validator/index unique cho transaction id, idempotency key, reference. OTP secret không lưu MongoDB. |
| Sổ cái | `ledger_entries.transaction_id`, `account_id`, `sequence`, `direction`, `amount`, `balance_before`, `balance_after`, `created_at` | Append-only theo contract; unique `(account_id, sequence)`, index `transaction_id` và `(account_id, created_at DESC)`. Tạo DEBIT/CREDIT tương ứng trong cùng session transaction. |
| Lịch sử/API list | `account_transaction_feed.account_id`, `transaction_id`, `direction`, `type`, `counterparty.account_no`, `counterparty.name`, `counterparty.bank_code`, `amount`, `fee`, `balance_after`, `status`, `note`, `reference`, `occurred_at` | Projection đọc CQRS; index `(account_id, occurred_at DESC, _id DESC)` và direction variant hỗ trợ phân trang/filter. Đây là nguồn phù hợp cho history và dashboard. |
| Idempotency | `transactions.idempotency_key` unique | Sơ đồ thể hiện UQ; cần quy định key scope (toàn hệ thống hay theo account) và cách replay response khi retry. |
| Audit/event | `outbox_events.event_id`, `aggregate_type/id`, `event_type`, `payload`, `status`, retry fields | Sơ đồ yêu cầu outbox nằm trong cùng write transaction; chi tiết payload là event-specific. |

## Giới hạn MVP còn lại

Migration bổ sung `note`, `fee`, `reference`, `status`, OTP expiry metadata và snapshots cần cho client vào validator transaction/feed. Backend hiện xử lý các field này như sau:

- `note`: được lưu ở transaction/feed và dùng trong tìm kiếm.
- `fee`: luôn 0 ở MVP.
- `reference`: mã `FT` + hậu tố UUID, unique trong collection; cần chốt format tra soát cuối cùng.
- `status`: migration cho phép `awaiting_otp`, `pending`, `success`, `failed`, `cancelled`; code hiện dùng `awaiting_otp`, `success`, `cancelled`.
- `counterparty` của feed có `account_no`, `name`, `bank_code`; chưa có tên ngân hàng hiển thị. Client có thể map `bankCode` sang label/logo.
- Search theo note, mã reference và amount cần các field tương ứng trong read projection hoặc query join/lookup phù hợp.
- Summary theo tháng cần đảm bảo feed có status/posted state hoặc API join sang transaction để chỉ cộng giao dịch thành công.
- Recent recipients cần lọc giao dịch chuyển đi thành công và dữ liệu recipient đầy đủ; xác nhận `type/status` cùng index hỗ trợ truy vấn.
- `accounts.balance.current` dùng Decimal128/int64 theo sơ đồ; API cần thống nhất cách encode JSON (số nguyên VND an toàn với phạm vi client) và currency.

Sau khi migration transaction được cập nhật, đối chiếu thêm các mục sau rồi sửa spec:

1. Chạy migration trên Mongo replica set và xác minh schema/index; local Compose dùng single-node replica set `rs0`.
2. Transfer/order và transaction cùng collection hay tách; `awaiting_otp` lưu ở đâu, TTL/attempt state thế nào.
3. Liên kết source/destination với account và customer để mọi query scope từ JWT.
4. Direction/feed entries được tạo cho cả bên gửi và nhận; idempotency scope, reference unique và confirm replay.
5. Chỗ lưu `note`, `fee`, `reference`, trạng thái chi tiết để đáp ứng response/search/summary của client.
6. Snapshot recipient và balanceAfter; cách encode tiền và timezone.
7. Mongo session transaction, outbox và projection cập nhật đồng bộ/trễ; API trả kết quả nào khi feed chưa cập nhật.

## Phần client đã nối

1. HTTP adapter dùng bearer token và parse `createdAt` thành `Date`.
2. Tra cứu, tạo lệnh/idempotency, xác nhận và gửi lại OTP đi qua API; không còn hiển thị OTP demo.
3. Dashboard, history, summary, cursor pagination và detail đọc từ API.
4. Số dư trong form chuyển được cập nhật cục bộ sau confirm; `/api/auth/me` chưa được refresh tự động nên số dư dashboard cần refresh profile/đăng nhập lại để đồng bộ.
5. Danh bạ tự lưu vẫn là RAM-only do backend chưa có contacts endpoint; người nhận gần đây lấy từ API.

## Câu hỏi cần chốt cùng migration/BE

- OTP giao dịch dùng service OTP hiện tại hay flow riêng? TTL và cooldown gửi lại?
- Hạn mức giao dịch/ngày và phí liên ngân hàng?
- Chuyển tiền liên ngân hàng là scope thực tế hay chỉ thiết kế tương lai? VietQR hiện chỉ cung cấp directory ngân hàng cho UI, không tự thực hiện thanh toán.
- Giao dịch có `pending` thực tế không; nếu có thì endpoint/status transition và cập nhật số dư thế nào?
- Format lỗi có hỗ trợ `code`, `attemptsLeft` và payload lặp confirm không?
- Dashboard `/me` được refresh sau confirm hay dùng `balanceAfter` từ response?

---

*Ngày soạn: 2026-09-30*  
*Trạng thái: MVP backend và client integration cho chuyển nội bộ đã triển khai; kiểm chứng runtime với Mongo replica set/email OTP còn pending.*
