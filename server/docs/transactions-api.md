# Contract giao dịch dành cho frontend

Contract này mô tả handler hiện tại trên nhánh transaction API. Swagger bật bằng
`SWAGGER_ENABLED=true`: UI `/swagger/`, spec `/openapi.json`. Chọn **Authorize** và
nhập JWT từ login. Base URL local mặc định: `http://localhost:8080`.

## Auth và cách đọc response

Tất cả 8 endpoint dưới đây cần `Authorization: Bearer <accessToken>`.
`POST /api/transfers` còn yêu cầu JWT có `isVerify=true`; sau KYC cần lấy token
được cập nhật theo luồng auth của ứng dụng. Source account luôn lấy từ JWT, không
truyền account người gửi trong body.

**Response thành công của transfer/transaction trả trực tiếp JSON**, không có
wrapper `{status,message,data}` của một số API auth/account. Ví dụ đọc
`body.transferId`, `body.items`, không phải `body.data.transferId`.

Lỗi nghiệp vụ của các endpoint này:

```json
{"error":"invalid otp","code":"INVALID_OTP","attemptsLeft":2}
```

Lỗi 401 từ JWT middleware dùng contract chung:

```json
{
  "status":401,
  "title":"Unauthorized",
  "errors":[{"code":"INVALID_TOKEN","detail":"invalid or expired token"}]
}
```

FE đọc mã lỗi bằng `body.code ?? body.errors?.[0]?.code`; dùng HTTP status để
phân nhánh. Không phụ thuộc chuỗi `error`/`detail` để quyết định hành vi.
Ngày giờ trả ISO/RFC3339: chuyển bằng `new Date(value)` ở adapter. Tiền là số
nguyên VND, không gửi chuỗi, số thập phân hoặc giá trị đã format dấu phân cách.

## Các endpoint

| Method + path | Input | Response thành công |
|---|---|---|
| `GET /api/accounts/lookup` | Query `bankCode=ANOMALY`, `accountNo` | 200: `{accountNo,name,bankCode}` |
| `POST /api/transfers` | Header `Idempotency-Key` UUID; body bên dưới | 201: `{transferId,status,recipient,amount,fee,note,otp}` |
| `POST /api/transfers/{transferId}/confirm` | Body `{"otp":"123456"}` | 200: `TransactionRecord` |
| `POST /api/transfers/{transferId}/otp/resend` | Không cần body | 200: `OTPInfo` |
| `GET /api/transfers/recent-recipients` | `limit`: mặc định 4, tối đa 10 | 200: `{items:[{accountNo,name,bankCode,lastTransferredAt}]}` |
| `GET /api/transactions` | Filters/cursor bên dưới | 200: `{items:TransactionRecord[],nextCursor:string\|null}` |
| `GET /api/transactions/summary` | `month=YYYY-MM` bắt buộc | 200: `{month,totalIn,totalOut}` |
| `GET /api/transactions/{id}` | `id` là UUID giao dịch (`transferId`) | 200: `TransactionRecord` |

Hiện chỉ chuyển nội bộ `ANOMALY`. Số tài khoản gồm 6–19 chữ số, không cho chuyển
vào chính tài khoản nguồn. Tài khoản mới có số tài khoản 14 chữ số.

### Tạo giao dịch và OTP

```http
POST /api/transfers
Authorization: Bearer <accessToken>
Content-Type: application/json
Idempotency-Key: 34297a72-9433-4adf-8958-868d5e50d197
```

```json
{
  "toBankCode":"ANOMALY",
  "toAccountNo":"12345678901234",
  "amount":100000,
  "note":"Chuyen tien"
}
```

`amount >= 1000`; `note` tối đa 100 ký tự, được trim, có thể để trống/không gửi.
Fee hiện tại là 0 VND. Lần tạo mới trả trạng thái `awaiting_otp`; chưa trừ tiền.

```json
{
  "transferId":"8bfbe3d4-742c-4602-b2bf-31ccf62de7ad",
  "status":"awaiting_otp",
  "recipient":{"accountNo":"12345678901234","name":"Nguyen Van B","bankCode":"ANOMALY"},
  "amount":100000,
  "fee":0,
  "note":"Chuyen tien",
  "otp":{
    "channel":"email",
    "maskedDestination":"n***@example.com",
    "expiresAt":"2026-10-07T05:05:00Z",
    "attemptsLeft":3,
    "resendAvailableAt":"2026-10-07T05:00:30Z"
  }
}
```

OTP gửi vào email người gửi, có thời hạn 5 phút, tối đa 3 lần thử và cooldown
resend 30 giây. Dùng timestamp trả về cho countdown, không tự đặt lại thời gian
khi re-render. Resend thành công trả cùng shape `otp` ở trên và reset số lần thử.
Nhập OTP như string để giữ số 0 đầu.

Tạo một UUID cho mỗi ý định chuyển tiền; giữ **cùng key + cùng body** khi retry do
mất mạng/timeout. Không tạo key mới cho mỗi lần bấm lại. Cùng key nhưng đổi người
nhận/amount/note trả 409 `IDEMPOTENCY_CONFLICT`. Retry có thể trả trạng thái giao
dịch cũ `success` hoặc `cancelled`; chỉ mở màn OTP khi `awaiting_otp`. Tạo lại
không gửi OTP lần nữa; dùng endpoint resend. `attemptsLeft` trong response create
retry hiện là 3, không phải số lượt còn lại được đọc từ Redis; cập nhật UI bằng
`INVALID_OTP.attemptsLeft` và response resend.

Sau confirm thành công, dùng ngay record trả về cho màn kết quả; retry confirm
với cùng `transferId` trả kết quả đã thành công mà không trừ thêm tiền. Số dư
được kiểm tra lại tại confirm, nên giao dịch có thể trả `INSUFFICIENT_FUNDS` dù
lần tạo trước đó đủ tiền.

### Record giao dịch

```json
{
  "id":"8bfbe3d4-742c-4602-b2bf-31ccf62de7ad",
  "reference":"FT62DE7AD1234",
  "direction":"out",
  "status":"success",
  "kind":"transfer",
  "amount":100000,
  "fee":0,
  "note":"Chuyen tien",
  "counterparty":{
    "name":"Nguyen Van B",
    "accountNo":"12345678901234",
    "bankCode":"ANOMALY",
    "bank":"AnomalyBank"
  },
  "createdAt":"2026-10-07T05:01:00Z",
  "balanceAfter":900000
}
```

- `direction`: `in` / `out` theo tài khoản đăng nhập.
- `status` trong history/detail: `success` / `failed` (`cancelled` được map thành
  `failed`). Feed chưa chứa giao dịch `awaiting_otp`; không dùng history để giữ
  trạng thái màn OTP.
- `createdAt` là thời điểm hoàn tất/hủy của feed, không phải lúc bắt đầu intent.
- `balanceAfter` là số dư sau giao dịch của người đang xem, vắng ở giao dịch hủy.
- `counterparty.bankCode` là mã ngân hàng; `bank` là tên hiển thị nếu có.
- `id` dùng gọi detail/confirm, `reference` chỉ dùng hiển thị/tra soát.
- Detail chỉ trả giao dịch thuộc người đăng nhập. Pending hoặc không có quyền
  xem trả 404 `TRANSACTION_NOT_FOUND`.

### History, phân trang và summary

```http
GET /api/transactions?direction=out&limit=20&q=chuyen&from=2026-10-01T00%3A00%3A00%2B07%3A00&to=2026-10-31T23%3A59%3A59%2B07%3A00
```

Dùng `URLSearchParams` để encode query, nhất là dấu `+` của timezone.

| Query | Quy tắc |
|---|---|
| `direction` | `all` (mặc định), `in`, `out` |
| `q` | Tìm tên/STK đối tác, note, reference, amount; không phân biệt hoa/thường/dấu |
| `from`, `to` | RFC3339, biên bao gồm cả hai đầu; tùy chọn |
| `limit` | Mặc định 20, tối đa 50; giá trị không hợp lệ dùng mặc định |
| `cursor` | Chuỗi opaque từ `nextCursor`, không tự decode hoặc dựng |

Danh sách sắp mới nhất trước. Giữ nguyên filters khi lấy trang kế tiếp; đổi
filters thì bỏ cursor và tải từ đầu. `nextCursor=null` là hết trang.

Summary bắt buộc `month=2026-10`, tính tháng theo UTC+07:00, chỉ cộng amount của
giao dịch thành công; `totalOut` chưa bao gồm fee. Đây là tổng tháng, không phải
tổng của trang hoặc filters history hiện tại. Recent recipients khử trùng người
nhận từ tối đa 100 dòng outgoing thành công gần nhất.

History, summary và recent recipients đọc Mongo projection nên có thể cập nhật
chậm hơn confirm. Confirm/detail đọc canonical state: FE giữ record confirm để
hiển thị ngay và refresh history sau đó; không coi việc record chưa có trong
history là confirm thất bại.

## Mã lỗi cần đồng bộ UI

| HTTP | Code | Cách xử lý |
|---|---|---|
| 401 | `MISSING_AUTH_HEADER`, `INVALID_TOKEN` | Lấy lại session/login theo luồng auth |
| 403 | `KYC_REQUIRED`, `ACCOUNT_INACTIVE` | Yêu cầu KYC/token mới hoặc thông báo tài khoản không hoạt động |
| 400 | `INVALID_REQUEST`, `UNSUPPORTED_BANK`, `INVALID_ACCOUNT_NO`, `INVALID_AMOUNT`, `INVALID_NOTE`, `INVALID_IDEMPOTENCY_KEY` | Sửa input |
| 400 | `INSUFFICIENT_FUNDS` | Thông báo không đủ số dư, không tự confirm lại |
| 400 | `INVALID_OTP` | Hiển thị `attemptsLeft`, cho thử lại nếu còn lượt |
| 400 | `INVALID_DIRECTION`, `INVALID_DATE`, `INVALID_CURSOR`, `INVALID_MONTH` | Sửa query; reset cursor nếu cần |
| 404 | `ACCOUNT_NOT_FOUND`, `TRANSFER_NOT_FOUND`, `TRANSACTION_NOT_FOUND` | Thông báo không tìm thấy/không có quyền |
| 409 | `IDEMPOTENCY_CONFLICT` | Không retry cùng key với body đã đổi |
| 409 | `TRANSFER_ALREADY_PROCESSED` | Không resend cho giao dịch đã kết thúc |
| 409 | `BALANCE_LIMIT_EXCEEDED` | Thông báo giới hạn số dư người nhận |
| 410 | `OTP_EXPIRED` | Giao dịch đã hủy; tạo ý định mới với UUID mới |
| 422 | `SELF_TRANSFER` | Chọn tài khoản nhận khác |
| 423 | `OTP_ATTEMPTS_EXCEEDED` | Giao dịch đã hủy; tạo ý định mới |
| 429 | `OTP_RESEND_TOO_SOON` | Chờ đến `resendAvailableAt` |
| 503 | `OTP_DELIVERY_FAILED` | Create đã yêu cầu hủy; báo gửi OTP thất bại |
| 500 | `INTERNAL_ERROR` | Kết quả có thể chưa rõ; retry create cùng key hoặc confirm cùng transferId |

## Trạng thái tích hợp FE

`client/mobile/src/features/transactions/api.ts` có adapter cho list/detail/summary.
`client/mobile/src/features/transfer/api/transfer.ts` hiện vẫn mock lookup và
submitTransfer; cần nối lookup/create/confirm/resend/recent-recipients theo contract
này. Dùng `requestJson` cho direct JSON; `requestSuccess` dành cho API có wrapper.
Lỗi transfer nằm ở `ApiError.body.code`, không nằm trong `ApiError.errors`;
adapter giữ tên field API, chuyển date và ánh xạ `bank ?? bankCode` sang tên ngân hàng hiển thị.
