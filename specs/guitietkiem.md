# API Spec — Gửi tiết kiệm

---

## 1. Tổng quan

| Thông tin | Chi tiết |
|-----------|----------|
| **Tính năng** | Khách hàng xem gói tiết kiệm, dự tính lãi, gửi tiền từ tài khoản thanh toán, nhận hợp đồng và tra cứu các khoản đang gửi |
| **Nguồn nghiệp vụ** | `QuyTrinhNghiepVu.docx` (UC-10), Use Case hệ thống Gửi Tiết Kiệm đã dự thảo (UC-S10), chính sách lãi suất V5 (dự thảo, quy tắc MVP đã chốt) |
| **Phạm vi** | Tiết kiệm VND có **7 kỳ hạn: 3, 6, 9, 12, 24, 36, 60 tháng** (không hỗ trợ 1 tháng); chung một biểu lãi cho mọi khu vực và mức thu nhập |
| **Mục đích** | Đặc tả API cho client và BE trước khi code, có request/response mẫu và quy tắc kiểm tra |
| **Hiện trạng client/server** | Chưa đối chiếu code trong repo; màn hình/endpoint bên dưới **đều là đề xuất** |
| **Trạng thái** | Draft v0.4 — đồng bộ các quyết định nghiệp vụ MVP; chờ triển khai |

### 1.1 Màn hình phía client (đề xuất)

| Route dự kiến | Màn hình | API dùng |
|---------------|----------|----------|
| `#/savings` | Danh sách sổ tiết kiệm đang có | 2.4 |
| `#/savings/open` | Chọn kỳ hạn, nhập số tiền, xem lãi suất | 2.1 |
| `#/savings/open` — xem trước | Tiền lãi dự kiến, ngày đáo hạn, tài khoản nguồn | 2.2 |
| `#/savings/open` — xác nhận | Xác nhận gửi, xem biên nhận | 2.3 |
| `#/savings/:id` | Chi tiết sổ tiết kiệm | 2.5 |

### 1.2 Quy ước chung

- `Authorization: Bearer <jwt>`; BE xác định khách hàng từ JWT, kiểm tra `sourceAccountId` thuộc user, không nhận `customerId` để xác định chủ sổ.
- Số tiền nguyên `int64` VND; thời điểm dùng RFC 3339 (có timezone). Kỳ hạn API dùng `termMonths` = `3`, `6`, `9`, `12`, `24`, `36`, `60` để tương thích thiết kế `savings_packages.term_options.months` và `savings_contracts.term.months`. UI hiển thị **3/6/9 tháng, 1/2/3/5 năm**.
- Lãi suất là `%/năm`; tỷ lệ trong bảng chính sách chỉ là **lãi suất của gói đang hiệu lực**. Sau khi mở sổ, lưu mức đã chốt vào `savings_contracts.interest_rate`.
- Mỗi sổ tối thiểu **1.000.000 VND**; dự thảo **không đặt trần tối đa cố định** theo từng sổ, nhưng phải kiểm tra số dư khả dụng và giới hạn vận hành/kỹ thuật. Khách hàng có thể mở nhiều sổ.
- API ghi tiền cần `Idempotency-Key: <uuid v4>`; mọi validate và số dư được BE kiểm tra lại khi xác nhận.
- Lỗi thống nhất `{ "error": "<message>", "code": "<ERROR_CODE>" }`.
- MVP **không hỗ trợ** gửi thêm vào sổ, rút trước hạn, rút một phần, tự tái tục hay lĩnh lãi định kỳ. Đến hạn chỉ tạo thông báo; tiền chỉ được hoàn khi khách bấm Rút và Backend xử lý thành công. Bản đầu **không yêu cầu OTP** khi mở sổ.

**Chính sách đã chốt để lập trình:** 7 kỳ hạn với lãi suất **4,5% / 4,8% / 5,0% / 5,2% / 5,5% / 5,8% / 6,0%/năm** tương ứng 3/6/9/12/24/36/60 tháng; thấp nhất 1.000.000 VND/sổ, không trần sản phẩm cố định, một người mở nhiều sổ. Biểu lãi là chính sách **giả lập AnomalyBank** (không phải bảng chính thức của MB). Lãi đơn theo **ngày thực/365**, làm tròn **HALF_UP** một lần tới 1 VND. Mở sổ không OTP; Quote mở sổ TTL **5 phút**.

**Quan hệ với rút tiết kiệm:** Tại `maturityAt`, scheduler chạy theo chu kỳ 1 phút, chỉ tạo thông báo in-app duy nhất cho sổ; không tự cộng gốc/lãi. Khách hàng chủ động chọn Rút tiền sau thời điểm đáo hạn (không cần chờ scheduler) và tiền được hoàn về đúng tài khoản nguồn. Hành vi thực thi được quy định trong `ruttietkiem_v0.2.md`. Phần Gia hạn trong thông báo chỉ là định hướng UI, **không triển khai trong MVP**.

---

## 2. Danh sách API đề xuất

| # | Method | Path | Mục đích |
|---|--------|------|----------|
| 2.1 | GET | `/api/savings/packages` | Danh sách kỳ hạn và lãi suất hiện hành |
| 2.2 | POST | `/api/savings/quotes` | Tính thử tiền lãi/ngày đáo hạn |
| 2.3 | POST | `/api/savings/contracts` | Xác nhận gửi tiền, mở hợp đồng tiết kiệm |
| 2.4 | GET | `/api/savings/contracts` | Danh sách hợp đồng tiết kiệm của user |
| 2.5 | GET | `/api/savings/contracts/{id}` | Chi tiết hợp đồng, lãi suất đã ký |

### 2.1 Danh sách sản phẩm và kỳ hạn

`GET /api/savings/packages`

**Response — 200**

```json
{
  "items": [
    {
      "packageId": "savings_standard",
      "name": "Tiết kiệm có kỳ hạn",
      "currency": "VND",
      "termOptions": [
        { "termMonths": 3, "termLabel": "3 tháng", "annualInterestRate": "4.5" },
        { "termMonths": 6, "termLabel": "6 tháng", "annualInterestRate": "4.8" },
        { "termMonths": 9, "termLabel": "9 tháng", "annualInterestRate": "5.0" },
        { "termMonths": 12, "termLabel": "1 năm", "annualInterestRate": "5.2" },
        { "termMonths": 24, "termLabel": "2 năm", "annualInterestRate": "5.5" },
        { "termMonths": 36, "termLabel": "3 năm", "annualInterestRate": "5.8" },
        { "termMonths": 60, "termLabel": "5 năm", "annualInterestRate": "6.0" }
      ],
      "minimumDeposit": 1000000,
      "effectiveAt": "2026-10-12T00:00:00+07:00"
    }
  ]
}
```

- **7 mức lãi suất đã chốt của bản MVP** tương ứng 3/6/9/12/24/36/60 tháng: **4,5% / 4,8% / 5,0% / 5,2% / 5,5% / 5,8% / 6,0%/năm**. Cả 7 mức đã được thống nhất để triển khai AnomalyBank.
- `minimumDeposit: 1000000` là mức tối thiểu đã chốt. Không bổ sung `maximumDeposit` vì chính sách không đặt trần cố định theo sổ; không có nghĩa là bỏ qua kiểm soát số tiền hoặc các hạn mức hệ thống.
- `effectiveAt` chỉ là timestamp minh họa hình dạng response, không phải ngày chính sách đã chính thức có hiệu lực.
- Không phân khu vực hay thu nhập, không gửi `regionGroup` vào endpoint.

**Error responses:** `404 SAVINGS_PACKAGE_NOT_FOUND` nếu một mã gói cụ thể không còn hiệu lực (khi mở rộng lọc theo gói).

### 2.2 Tính thử tiền gửi và tiền lãi

`POST /api/savings/quotes`

**Request**

```json
{
  "packageId": "savings_standard",
  "sourceAccountId": "acc_001",
  "amount": 100000000,
  "termMonths": 12
}
```

**Response — 200**

```json
{
  "quoteId": "sav_q_001",
  "principal": 100000000,
  "currency": "VND",
  "termMonths": 12,
  "annualInterestRate": "5.2",
  "expectedMaturityDate": "2027-10-12",
  "estimatedInterest": 5200000,
  "estimatedTotalAtMaturity": 105200000,
  "quoteExpiresAt": "2026-10-12T09:15:00+07:00",
  "isFinal": false
}
```

- `5.200.000` đồng là kết quả theo quy tắc đã chốt: từ 12/10/2026 đến 12/10/2027 là 365 ngày, tính 100.000.000 × 5,2% × 365/365. Tổng lãi tính theo số ngày lịch thực/365 kể cả năm nhuận, làm tròn kết quả một lần theo HALF_UP đến 1 VND.
- MVP **không cho phép rút trước hạn**. Sổ giữ nguyên lãi suất hợp đồng đến đáo hạn và chờ khách yêu cầu rút.
- Quote có hiệu lực **5 phút** kể từ thời điểm tạo, không giữ chỗ số dư. Quá TTL trả `QUOTE_EXPIRED`; khi xác nhận vẫn kiểm tra lại số dư, quyền sở hữu và chính sách.

**Validate**

| Trường | Quy tắc |
|--------|---------|
| `packageId` | Có hiệu lực và hỗ trợ tiền gửi VND |
| `sourceAccountId` | Tài khoản thanh toán thuộc chủ tài khoản hiện tại, còn hoạt động |
| `amount` | Số nguyên VND, **>= 1.000.000**, không vượt số dư khả dụng và các hạn mức an toàn/vận hành thực tế (dự thảo không đặt trần tiền gửi sản phẩm cố định) |
| `termMonths` | Một trong `3`, `6`, `9`, `12`, `24`, `36`, `60` |

**Error responses**

| HTTP | code | Client hiển thị |
|------|------|-----------------|
| 400 | `INVALID_AMOUNT` | "Số tiền gửi không hợp lệ." |
| 400 | `UNSUPPORTED_TERM` | "Kỳ hạn này chưa được hỗ trợ." |
| 400 | `INSUFFICIENT_FUNDS` | "Số dư không đủ để gửi tiết kiệm." |
| 422 | `BELOW_MINIMUM_DEPOSIT` | "Số tiền gửi tối thiểu là 1.000.000 VND." |
| 422 | `INVALID_SOURCE_ACCOUNT` | "Tài khoản thanh toán không hợp lệ." |
| 404 | `SAVINGS_PACKAGE_NOT_FOUND` | "Không tìm thấy gói tiết kiệm." |

### 2.3 Xác nhận gửi tiết kiệm

`POST /api/savings/contracts`

**Headers:** `Idempotency-Key: <uuid v4>`

**Request**

```json
{
  "quoteId": "sav_q_001",
  "sourceAccountId": "acc_001",
  "confirm": true
}
```

**Response — 201**

```json
{
  "savingsId": "sav_001",
  "savingsNo": "SV20261012001",
  "transactionId": "tx_sav_deposit_001",
  "status": "active",
  "principal": 100000000,
  "annualInterestRate": "5.2",
  "termMonths": 12,
  "openedAt": "2026-10-12T09:10:00+07:00",
  "maturityAt": "2027-10-12T09:10:00+07:00",
  "sourceAccountId": "acc_001"
}
```

**BE bắt buộc:**

1. Đọc quote hợp lệ và kiểm tra lại user, gói, lãi suất, tài khoản nguồn, số dư khả dụng.
2. Ghi nợ tài khoản thanh toán đúng một lần, ghi nhận transaction/ledger, tạo `savings_contracts` với gốc, ngày mở, đáo hạn và lãi suất snapshot.
3. Phát sự kiện/audit phục vụ replay và truy vết, nhưng **không** đưa thông tin cá nhân nhạy cảm vào payload không cần thiết.
4. Nếu xử lý bất thành, không được trả kết quả `active` khi tiền chưa được hạch toán hợp lệ. Cần cơ chế xử lý trạng thái trung gian/khôi phục theo kiến trúc CQRS/Event Sourcing.
5. Retry cùng `Idempotency-Key` không được trừ tiền hoặc mở sổ hai lần.

**Error responses**

| HTTP | code | Khi nào |
|------|------|---------|
| 400 | `CONFIRMATION_REQUIRED` | `confirm` khác `true` |
| 400 | `INSUFFICIENT_FUNDS` | Số dư thay đổi kể từ lúc xem quote |
| 404 | `QUOTE_NOT_FOUND` | Không tìm thấy quote hợp lệ |
| 409 | `IDEMPOTENCY_CONFLICT` | Cùng key nhưng khác request |
| 409 | `SAVINGS_ALREADY_CREATED` | Quote/request đã xử lý, trả kết quả cũ phù hợp |
| 410 | `QUOTE_EXPIRED` | Quote hết hạn, cần xem lại lãi suất |
| 503 | `SAVINGS_OPEN_UNAVAILABLE` | Hệ thống chưa thể xác nhận kết quả an toàn |

> MVP không yêu cầu OTP bổ sung khi mở sổ. `confirm: true` chỉ là một bước xác nhận, KHÔNG thay cho JWT, kiểm tra quyền sở hữu, số dư, tính hợp lệ Quote và Idempotency. Nếu bổ sung OTP trong tương lai cần version lại đặc tả.

### 2.4 Danh sách sổ tiết kiệm

`GET /api/savings/contracts?status=active&limit=20&cursor=`

**Response — 200**

```json
{
  "items": [
    {
      "savingsId": "sav_001",
      "savingsNo": "SV20261012001",
      "principal": 100000000,
      "annualInterestRate": "5.2",
      "termMonths": 12,
      "openedAt": "2026-10-12T09:10:00+07:00",
      "maturityAt": "2027-10-12T09:10:00+07:00",
      "status": "active"
    }
  ],
  "nextCursor": null
}
```

- Có thể lọc `active` / `matured` / `settled` / `all`; BE phải lọc theo user từ JWT. `matured` nghĩa là đã đến thời điểm đáo hạn nhưng chưa tất toán; quyền rút dựa trên `maturityAt`, không phụ thuộc projection đã kịp gán `matured` hay chưa.
- Tối đa 50 bản ghi/trang, sắp xếp theo `openedAt` mới nhất trước.

### 2.5 Chi tiết sổ tiết kiệm

`GET /api/savings/contracts/sav_001`

**Response — 200**

```json
{
  "savingsId": "sav_001",
  "savingsNo": "SV20261012001",
  "principal": 100000000,
  "annualInterestRate": "5.2",
  "termMonths": 12,
  "earlyWithdrawalRateAtOpening": "0.2",
  "openedAt": "2026-10-12T09:10:00+07:00",
  "maturityAt": "2027-10-12T09:10:00+07:00",
  "status": "active",
  "settlement": null
}
```

`earlyWithdrawalRateAtOpening` được giữ trong response theo schema thiết kế lịch sử, nhưng **không dùng trong MVP** vì MVP không cho rút trước hạn. Không cần sửa hai file JSON phân tích/thiết kế.

**Error responses:** `404 SAVINGS_NOT_FOUND` khi không tồn tại hoặc không thuộc user.

---

## 3. Schema và liên hệ MongoDB

| API field | Collection / trường | Vai trò |
|-----------|---------------------|---------|
| `packageId`, `termOptions` | `savings_packages`, `term_options[]` | Danh mục kỳ hạn/lãi suất mới |
| `savingsId`, `savingsNo` | `savings_contracts._id`, `savings_no` | Hợp đồng tiết kiệm |
| `principal`, `termMonths` | `savings_contracts.principal`, `term.months` | Gốc và kỳ hạn |
| `annualInterestRate` | `savings_contracts.interest_rate` | Snapshot lãi suất khi mở |
| `earlyWithdrawalRateAtOpening` | `savings_contracts.early_withdrawal_rate` | Snapshot tham khảo, không thay quy tắc tại lúc rút |
| `sourceAccountId` | `savings_contracts.source_account_id` | TK đã trích tiền |
| `transactionId` | `transactions`, `ledger_entries` | Hạch toán tiền gửi |
| Lịch sử hiệu lực biểu lãi | `interest_rate_policies` (đề xuất trong Word) | Không thay JSON hiện tại |

---

## 4. Quy tắc nghiệp vụ (đã thống nhất cho MVP)

1. Kỳ hạn **3/6/9/12/24/36/60 tháng** với lãi suất **4,5/4,8/5,0/5,2/5,5/5,8/6,0%/năm**. Không có 1 tháng, không phân biệt khu vực/thu nhập; lãi suất cố định theo hợp đồng đã mở.
2. Tiền gửi từ **1.000.000 VND/sổ**, không có trần cố định theo sản phẩm. Một khách hàng mở nhiều sổ; nguồn tiền là tài khoản thanh toán chính chủ đủ số dư; backend kiểm tra lại tại commit.
3. **Lãi đơn**: `interest = ROUND_HALF_UP(principal * annualInterestRate/100 * actualCalendarDays / 365)` (VND). Ngày thực gửi = chênh lệch ngày lịch theo thời điểm mở và ngày đáo hạn, không mặc định 30 ngày/tháng, năm nhuận vẫn chia 365. Chỉ làm tròn sau cùng.
4. **Maturity:** cộng số tháng theo lịch vào ngày mở, giữ nguyên giờ/phút/giây; nếu tháng đích thiếu ngày tương ứng thì chọn cuối tháng. Ví dụ 31/01/2027 + 3 tháng = 30/04/2027, giữ nguyên giờ.
5. Quote mở sổ TTL **5 phút**, không khóa tiền; API xác nhận không OTP nhưng bắt buộc JWT, xác nhận người dùng, validate Quote và `Idempotency-Key`.
6. Tiền lãi và gốc được giữ trong sổ đến khi khách **bấm Rút tiền** sau đáo hạn. Không tự hoàn tiền. Scheduler 1 phút chỉ phát hiện đến hạn và tạo **một** thông báo in-app cho mỗi sổ. Backend rút dựa trên `now >= maturityAt` và trạng thái chưa tất toán, không phải chờ Scheduler.
7. Nếu chưa rút, sổ chờ vô thời hạn trong phạm vi MVP, không sinh thêm lãi sau hạn. Không triển khai rút trước hạn, rút một phần, tự gia hạn, gửi thêm tiền, hay gọi nhắc sau 3 ngày.
8. Lệnh ghi nợ mở sổ, event sourcing, ledger, projector, cache và idempotency phải đảm bảo không trừ tiền hai lần hay báo `active` trước khi giao dịch đã hoàn tất an toàn.
9. Rút đúng/sau hạn chỉ trả gốc và lãi cố định tính đến `maturityAt` về chính `sourceAccountId`. Nếu tài khoản không đủ điều kiện nhận hoặc lỗi hệ thống, giữ trạng thái chờ xử lý, có cơ chế retry và chống hoàn tiền trùng.
10. Thông báo in-app cần được lưu để Client truy vấn sau này; hiện chưa làm UI, Push Notification và logic Gia hạn. API thông báo là một luồng tích hợp riêng, sẽ đối chiếu cơ chế notification hiện có của Server trước khi đặt route.

**Domain events gợi ý (tên chờ đối chiếu convention repo):** `SavingsContractOpened`, `SavingsMatured`, `SavingsMaturityNoticeCreated`, `SavingsContractSettled`. Không coi Quote là event bắt buộc nếu chưa có thay đổi trạng thái nghiệp vụ.

---

## 5. Mapping client ↔ API

| Hành động trên app | API |
|-------------------|-----|
| Chọn gói/kỳ hạn | `GET /api/savings/packages` |
| Nhập tiền và xem lãi ước tính | `POST /api/savings/quotes` |
| Bấm "Xác nhận gửi" | `POST /api/savings/contracts` |
| Xem danh sách sổ | `GET /api/savings/contracts` |
| Xem chi tiết sổ | `GET /api/savings/contracts/{id}` |

---

## 6. Quyết định đã chốt và những phần sẽ xác định khi code

| Nội dung | Quyết định |
|----------|------------|
| Lãi suất 7 kỳ hạn | Đã chốt đúng bảng ở mục 2.1 |
| Lãi đơn, ngày thực / 365, năm nhuận | Đã chốt; làm tròn HALF_UP một lần đến 1 VND |
| Ngày đáo hạn | Cộng tháng theo lịch; ngày thiếu lấy cuối tháng; giữ nguyên giờ/phút/giây |
| Mức gửi | Tối thiểu 1.000.000 VND/sổ; không trần cố định; mở nhiều sổ |
| OTP mở sổ | Không dùng trong MVP |
| Quote mở sổ | TTL 5 phút |
| Tất toán | Chỉ khi khách bấm Rút tiền tại/sau `maturityAt`; không tự hoàn tiền |
| Scheduler | Mỗi phút, tạo thông báo in-app một lần/sổ; không push |
| Gia hạn / rút trước hạn / rút một phần | Chưa triển khai |
| Tích hợp ML/fraud/Kafka | Không mở rộng ngoài phạm vi module Savings và các điểm tích hợp sẵn; nhóm khác phụ trách Kafka |

**Kỹ thuật cần kiểm tra với source trước khi hiện thực:** transaction và event atomically nhất quán; cách đồng bộ với projection và xử lý lag, khóa đồng thời/idempotency, notification storage/repository và retry. Đây là chi tiết thiết kế kỹ thuật, không thay đổi nghiệp vụ đã chốt.

---

*Phiên bản: 0.4 — đồng bộ nghiệp vụ gửi và đáo hạn đã chốt cho MVP; chưa code.*  
*Bản cập nhật: 2026-10-24 (bản gốc: 2026-10-12).*  
*Trạng thái: Draft để review kỹ thuật trước khi triển khai GĐ1.*
