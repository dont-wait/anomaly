# API Spec — Rút tiết kiệm (MVP: rút toàn bộ tại/sau đáo hạn)

---

## 1. Tổng quan

| Thông tin | Chi tiết |
|-----------|----------|
| **Chức năng** | Khách chủ động bấm Rút tiền sau khi sổ đáo hạn; Backend hoàn toàn bộ gốc + lãi vào chính tài khoản thanh toán đã dùng mở sổ |
| **Nguồn nghiệp vụ** | `QuyTrinhNghiepVu.docx` (UC-11), `QuyTrinhHeThong.docx` (UC-S11), chính sách lãi suất V5 và các quyết định MVP đã chốt |
| **Quan hệ với Gửi tiết kiệm** | Dùng hợp đồng và lãi suất đã lưu từ `guitietkiem_v0.4.md` |
| **Phạm vi** | Chỉ rút **toàn bộ** đúng hoặc sau thời điểm đáo hạn; không rút trước hạn, không rút một phần, không gia hạn, không phạt |
| **Trạng thái** | Draft v0.2 — đồng bộ nghiệp vụ đã chốt; chưa code |

**Đặc biệt:** Đến ngày đáo hạn, hệ thống **KHÔNG tự chuyển gốc + lãi về số dư**. Scheduler quét mỗi phút và tạo thông báo in-app một lần/sổ, nội dung cho khách lựa chọn “Rút tiền” hoặc “Gia hạn”, nhưng **chỉ triển khai Rút tiền** trong MVP. Không phải chờ Scheduler hoặc đọc thông báo mới được rút: Backend cho phép khi `now >= maturityAt` và hợp đồng chưa tất toán.

### 1.1 Quy ước chung

- `Authorization: Bearer <jwt>` cho mọi API. Server xác định chủ sổ từ JWT và dữ liệu đáng tin cậy, không nhận `customerId` do Client tự truyền.
- Không yêu cầu OTP khi rút trong MVP, nhưng vẫn bắt buộc xác nhận `confirm: true` và kiểm tra quyền sở hữu, trạng thái sổ, ngày đáo hạn.
- VND biểu diễn bằng số nguyên `int64` tại API, thời gian theo RFC 3339; lãi suất theo `%/năm`, tính nội bộ bằng số thập phân chính xác, tránh `float` thiếu chính xác.
- `Idempotency-Key: <uuid v4>` bắt buộc cho API tất toán. Lặp lại cùng yêu cầu không được cộng tiền lần hai.
- Tiền trả về **chỉ** tài khoản `source_account_id` của hợp đồng mở sổ; không chọn tài khoản khác.
- Lãi tại thời điểm rút luôn là số lãi tính đến `maturityAt`, không phát sinh thêm trong thời gian chờ khách thao tác.
- Lỗi chung: `{ "error": "<message>", "code": "<ERROR_CODE>" }`.

## 2. Danh sách API

| # | Method | Path | Mục đích |
|---|--------|------|----------|
| 2.1 | POST | `/api/savings/contracts/{id}/withdrawal-quotes` | Xem gốc, lãi và tổng tiền nhận trước khi rút |
| 2.2 | POST | `/api/savings/contracts/{id}/withdrawals` | Khách xác nhận rút, hoàn tiền về tài khoản nguồn |
| 2.3 | GET | `/api/savings/contracts/{id}/settlement` | Xem biên nhận đã tất toán |

`GET /api/savings/contracts` và `GET /api/savings/contracts/{id}` nằm trong Spec Gửi tiết kiệm, không triển khai trùng. API thông báo đáo hạn được thiết kế theo cơ chế notification chung của Server, không tự đoán route mới trước khi kiểm tra source.

### 2.1 Tính trước số tiền được rút

`POST /api/savings/contracts/sav_001/withdrawal-quotes`

**Request**

```json
{
  "withdrawalType": "full"
}
```

**Response — 200 (minh họa rút ngay lúc đáo hạn)**

```json
{
  "quoteId": "sav_wq_001",
  "savingsId": "sav_001",
  "withdrawalType": "full",
  "settlementType": "at_maturity",
  "principal": 100000000,
  "contractAnnualInterestRate": "5.2",
  "appliedAnnualInterestRate": "5.2",
  "interestAmount": 5200000,
  "totalPayout": 105200000,
  "maturityAt": "2027-10-12T09:10:00+07:00",
  "calculatedAt": "2027-10-12T09:12:00+07:00",
  "destinationAccountId": "acc_001",
  "quoteExpiresAt": "2027-10-12T09:17:00+07:00",
  "requiresEarlyWithdrawalConfirmation": false
}
```

**Giải thích ví dụ:** Mở sổ 100 triệu vào `2026-10-12T09:10:00+07:00`, kỳ hạn 12 tháng, lãi suất hợp đồng 5,2%/năm, chênh lệch 365 ngày; lãi = 100.000.000 × 5,2% × 365/365 = **5.200.000 VND**, tổng tiền về tài khoản **105.200.000 VND**. Nếu rút một tháng sau đáo hạn thì **vẫn là 105.200.000 VND**. Quote này minh họa TTL 5 phút; cần kiểm tra/khóa quote và trạng thái tại thời điểm xác nhận.

**Quy tắc kiểm tra:**

- Sổ tồn tại, thuộc khách từ JWT, chưa `settled` hoặc bị tạm giữ hợp lệ.
- `now >= maturityAt` là điều kiện quyết định được rút; **không bắt buộc** projection đã kịp chuyển `active` → `matured`.
- Chỉ chấp nhận `withdrawalType: "full"`; cấm rút trước hạn và rút một phần.
- Không lấy gốc, lãi suất hay tài khoản nhận do Client cung cấp; Backend đọc từ hợp đồng.
- Nếu tài khoản nguồn không đủ điều kiện nhận tiền thì báo không thể hoàn tiền an toàn; hợp đồng chưa tất toán.

**Error responses:**

| HTTP | code | Khi nào |
|------|------|---------|
| 400 | `UNSUPPORTED_WITHDRAWAL_TYPE` | Khác `full` |
| 404 | `SAVINGS_NOT_FOUND` | Không tồn tại/không thuộc khách |
| 409 | `SAVINGS_ALREADY_SETTLED` | Đã tất toán |
| 422 | `SAVINGS_NOT_MATURED` | `now < maturityAt` (không hỗ trợ rút trước hạn) |
| 423 | `SAVINGS_LOCKED` | Sổ bị tạm giữ/khóa hợp lệ |
| 422 | `DESTINATION_ACCOUNT_UNAVAILABLE` | Tài khoản nguồn không thể nhận tiền |

### 2.2 Xác nhận rút tiền

`POST /api/savings/contracts/sav_001/withdrawals`

**Headers:** `Idempotency-Key: <uuid v4>`

**Request**

```json
{
  "quoteId": "sav_wq_001",
  "confirm": true
}
```

**Response — 200**

```json
{
  "savingsId": "sav_001",
  "savingsNo": "SV20261012001",
  "transactionId": "tx_sav_withdraw_001",
  "status": "settled",
  "settlementType": "at_maturity",
  "principal": 100000000,
  "interestAmount": 5200000,
  "totalPayout": 105200000,
  "appliedAnnualInterestRate": "5.2",
  "destinationAccountId": "acc_001",
  "settledAt": "2027-10-12T09:12:00+07:00"
}
```

**Backend bắt buộc:**

1. Xác thực JWT, quyền sở hữu sổ, `confirm: true` và `now >= maturityAt`, kiểm tra sổ chưa tất toán.
2. Đọc và xác nhận quote hợp lệ, đúng chủ và đúng sổ; đối chiếu lại lãi đã chốt, số dư hạch toán, tình trạng tài khoản nhận.
3. Dùng lãi suất hợp đồng lúc mở, lãi đơn ngày thực/365, **ROUND_HALF_UP** đến 1 VND một lần. Không tính thêm lãi sau `maturityAt`.
4. Chỉ hoàn **toàn bộ gốc và lãi** vào `source_account_id`; không cho Client thay đích.
5. Ghi Banking Event/ledger và kết quả hợp đồng một cách nhất quán; đảm bảo không có tình trạng cộng tiền thành công nhưng vẫn cho tất toán lại.
6. Retry với cùng `Idempotency-Key` hoặc hai request đồng thời không được cộng tiền hai lần. Khi trạng thái chưa rõ phải trả trạng thái chờ thay vì thành công giả.
7. Chỉ báo `settled` khi hệ thống xác định đã hoàn tiền thành công; nếu tài khoản bị khóa/đóng không thể nhận thì giữ trạng thái chờ. Tự retry sự cố tạm thời và có log/chẩn đoán.
8. MVP **không OTP**, không có `confirmEarlyWithdrawal`, không có lệnh rút trước hạn hay tái tục.

**Error responses:**

| HTTP | code | Khi nào |
|------|------|---------|
| 400 | `CONFIRMATION_REQUIRED` | `confirm != true` |
| 404 | `SAVINGS_NOT_FOUND` | Không tồn tại/không thuộc khách |
| 409 | `SAVINGS_ALREADY_SETTLED` | Tất toán trước đó |
| 409 | `IDEMPOTENCY_CONFLICT` | Cùng key nhưng khác payload |
| 410 | `QUOTE_EXPIRED` | Quote rút hết TTL |
| 422 | `SAVINGS_NOT_MATURED` | Chưa tới `maturityAt` |
| 422 | `DESTINATION_ACCOUNT_UNAVAILABLE` | Tài khoản nguồn chưa đủ điều kiện nhận |
| 423 | `SAVINGS_LOCKED` | Tạm giữ hợp lệ |
| 503 | `SETTLEMENT_UNAVAILABLE` | Chưa xác nhận hoàn tiền an toàn |

### 2.3 Xem biên nhận tất toán

`GET /api/savings/contracts/sav_001/settlement`

**Response — 200**

```json
{
  "savingsId": "sav_001",
  "savingsNo": "SV20261012001",
  "status": "settled",
  "settlement": {
    "type": "at_maturity",
    "principal": 100000000,
    "interestAmount": 5200000,
    "totalAmount": 105200000,
    "transactionId": "tx_sav_withdraw_001",
    "settledAt": "2027-10-12T09:12:00+07:00"
  }
}
```

**Error:** `404 SAVINGS_NOT_FOUND` (không thuộc khách/không tồn tại); `409 SAVINGS_NOT_SETTLED` (chưa có tất toán).

## 3. Scheduler, notification và trạng thái

- Scheduler quét các sổ tới `maturityAt` **mỗi 1 phút**, đảm bảo tạo **một** thông báo in-app cho mỗi sổ; không push, không nhắc hằng ngày.
- Dự kiến trạng thái: `active` (chưa hạn), `matured` (đến hạn, chờ rút), `settled` (hoàn thành). Read Model có thể trễ, nên lệnh Rút cần đánh giá `now >= maturityAt` trực tiếp dựa trên dữ liệu nghiệp vụ tin cậy.
- Sau đáo hạn mà khách không rút: sổ vẫn chờ, không có thêm lãi và không gia hạn. Ý tưởng ngân hàng gọi sau 3 ngày là **ngoài MVP**.
- Khi khách rút trước lúc notification được tạo, không để Scheduler thay đổi sổ đã `settled` hoặc tạo thông báo sai trạng thái.
- Thông báo nên lưu theo định danh sổ + loại sự kiện/phiên bản để deduplicate. Tận dụng hạ tầng notification nếu Server đã có, nếu chưa thì bổ sung tối thiểu cơ chế lưu và API đọc, tránh sửa Kafka.

## 4. Schema và liên hệ MongoDB/EventStoreDB

| Mục | Nguồn dữ liệu | Vai trò |
|-----|----------------|---------|
| `savingsId`, `customer_id` | `savings_contracts` | Quyền sở hữu |
| `principal`, `maturity_at`, `interest_rate` | `savings_contracts` | Căn cứ tính lãi |
| `source_account_id` | `savings_contracts` | Duy nhất tài khoản được hoàn tiền |
| `settlement` | `savings_contracts.settlement` | Kết quả sau khi hoàn tiền |
| Ghi có/tất toán | EventStoreDB + `transactions`, `ledger_entries`, `accounts` projections | Source of Truth và read model |
| Notification | Kho lưu thông báo nội bộ (sẽ chọn theo source) | Xem lại và chống gửi trùng |

Hai JSON phân tích/thiết kế hiện có **không bị sửa**. Nếu cần collection cho thông báo hoặc trạng thái xử lý trung gian, đề xuất ở tài liệu kỹ thuật/migration trong nhánh Savings, không tự thay đổi JSON nguồn.

## 5. Mapping Client tương lai ↔ API

| Hành động | API |
|-----------|-----|
| Xem sổ đã đến hạn | `GET /api/savings/contracts` hoặc `GET /api/savings/contracts/{id}` |
| Xem trước số tiền nhận | `POST /api/savings/contracts/{id}/withdrawal-quotes` |
| Bấm xác nhận Rút tiền | `POST /api/savings/contracts/{id}/withdrawals` |
| Xem biên nhận | `GET /api/savings/contracts/{id}/settlement` |
| Xem thông báo | API thông báo in-app sẽ xác định theo Server hiện có |

## 6. Các điểm kỹ thuật cần làm rõ khi triển khai

- **Quote rút TTL:** Ví dụ bên trên sử dụng 5 phút để đồng bộ trải nghiệm, nhưng user mới chốt TTL 5 phút cho **Quote gửi**. Chưa coi TTL Quote rút là quyết định nghiệp vụ cuối cùng; có thể lấy 5 phút làm lựa chọn triển khai nếu nhóm đồng ý.
- **Tài khoản nguồn đóng vĩnh viễn:** giữ quyền đòi tiền/trạng thái cần xử lý thủ công; không tự chuyển sang tài khoản khác.
- **Concurrency / Event Sourcing:** chọn vị trí kiểm soát idempotency và giao dịch hạch toán từ thực tế Go Server để ngăn double-credit ngay cả khi projection lag, worker restart hoặc response timeout.
- **Fraud pipeline:** không thay đổi Kafka hoặc phần việc của thành viên khác trong đợt này.

---

*Phiên bản: 0.2 — chỉ rút toàn bộ tại/sau đáo hạn sau khi khách bấm xác nhận.*  
*Ngày cập nhật: 2026-10-24.*  
*Trạng thái: Draft, chờ review kỹ thuật và kiểm thử.*
