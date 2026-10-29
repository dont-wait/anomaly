# API Spec — Vay vốn cá nhân

---

## 1. Tổng quan

| Thông tin | Chi tiết |
|-----------|----------|
| **Tính năng** | Khách hàng xem chính sách vay, dự tính khoản vay, nộp hồ sơ, theo dõi xét duyệt, xác nhận hợp đồng và nhận giải ngân |
| **Phạm vi** | Vay cá nhân dành cho khách hàng có thu nhập ổn định từ lương; **không** triển khai thẻ tín dụng, sản phẩm tự doanh hay vay có tài sản bảo đảm trong bản đầu |
| **Nguồn nghiệp vụ** | `QuyTrinhHeThong.docx` (UC-S06, UC-S07), `QuyTrinhNghiepVu.docx` (UC-04, UC-06), `chinhSach_LaiSuat_NganHang_DaCapNhat.docx` (V2) |
| **Mục đích tài liệu** | Đề xuất API, JSON request/response, validation và các trường hợp lỗi để client/BE review trước khi code |
| **Hiện trạng client/server** | **Chưa xác minh trong repo.** Các route và API dưới đây đều là **đề xuất**, không khẳng định đã tồn tại |
| **Trạng thái** | Draft v0.1 — chờ nhóm review |

### 1.1 Màn hình phía client (đề xuất)

| Route dự kiến | Màn hình | API dùng |
|---------------|----------|----------|
| `#/loans/apply` | Giới thiệu khoản vay, kỳ hạn, điều kiện, nhập số tiền | 2.1, 2.2 |
| `#/loans/apply/documents` | Nhập thông tin hồ sơ, tải chứng từ | 2.3, 2.4 |
| `#/loans/apply/review` | Kiểm tra hồ sơ và gửi xét duyệt | 2.5 |
| `#/loans/applications` | Danh sách hồ sơ đã gửi | 2.6 |
| `#/loans/applications/:id` | Trạng thái thẩm định, đề nghị vay, xác nhận hợp đồng | 2.7, 2.8 |
| `#/loans` và `#/loans/:id` | Danh sách và chi tiết khoản vay đã giải ngân | 2.9, 2.10 |

> Tên route chỉ nhằm mô tả luồng màn hình; khi ghép UI hiện có, đổi route chứ không tự xây giao diện mới nếu không cần.

### 1.2 Quy ước chung

- Yêu cầu `Authorization: Bearer <jwt>`; định danh khách hàng lấy từ token, **không nhận `customerId` do client tự gửi**.
- Số tiền VND: số nguyên `int64`, không lưu hoặc truyền bằng `float`.
- Thời gian: RFC 3339 có timezone, ví dụ `"2026-10-12T09:00:00+07:00"`.
- Mọi mức lãi đều có đơn vị `%/năm`. Lãi suất trong hồ sơ/hợp đồng là **snapshot** tại lúc được chốt, không tự thay đổi khi sửa biểu lãi suất.
- Lỗi dùng `{ "error": "<message>", "code": "<ERROR_CODE>" }` (trường `code` là đề xuất tương thích với `transaction-spec.md`).
- API thay đổi trạng thái/tạo dòng tiền phải có `Idempotency-Key: <uuid v4>`; BE kiểm tra quyền sở hữu, trạng thái, hạn mức và chống xử lý trùng.
- Hồ sơ định danh/chứng từ riêng tư: BE lưu file trong private object storage, MongoDB chỉ lưu metadata và `storage_key`; **không trả đường dẫn bucket công khai**.

---

## 2. Danh sách API đề xuất

| # | Method | Path | Mục đích |
|---|--------|------|----------|
| 2.1 | GET | `/api/loans/packages` | Danh sách sản phẩm/kỳ hạn vay đang hỗ trợ |
| 2.2 | POST | `/api/loans/quotes` | Tính thử lãi suất và lịch thanh toán dự kiến |
| 2.3 | POST | `/api/loan-applications` | Tạo hồ sơ vay nháp |
| 2.4 | POST | `/api/loan-applications/{id}/documents` | Tải chứng từ vào hồ sơ |
| 2.5 | POST | `/api/loan-applications/{id}/submit` | Gửi hồ sơ để thẩm định |
| 2.6 | GET | `/api/loan-applications` | Danh sách hồ sơ của chính khách hàng |
| 2.7 | GET | `/api/loan-applications/{id}` | Trạng thái và kết quả xét duyệt |
| 2.8 | POST | `/api/loan-applications/{id}/accept-offer` | Chấp nhận đề nghị vay / khởi tạo bước ký hợp đồng |
| 2.9 | GET | `/api/loans` | Các khoản vay đã giải ngân |
| 2.10 | GET | `/api/loans/{id}` | Chi tiết khoản vay và thông tin hợp đồng |

### 2.1 Danh sách sản phẩm vay

`GET /api/loans/packages`

**Response — 200**

```json
{
  "items": [
    {
      "packageId": "loan_pkg_personal",
      "name": "Vay vốn cá nhân",
      "currency": "VND",
      "availableTermMonths": [12],
      "incomeSource": "salary",
      "requiresKyc": true
    }
  ]
}
```

- V2 **mới chốt biểu lãi cho kỳ hạn 1 năm**. Kỳ hạn 24/36 tháng chỉ thêm khi nhóm chốt chính sách.
- `minAmount`/`maxAmount` chưa có mức thống nhất, không hard-code trong ví dụ.

### 2.2 Tính thử khoản vay

`POST /api/loans/quotes`

**Request**

```json
{
  "packageId": "loan_pkg_personal",
  "amount": 50000000,
  "termMonths": 12
}
```

**Response — 200** (số tiền trong ví dụ là minh họa cấu trúc; BE phải tính bằng thuật toán thống nhất)

```json
{
  "quoteId": "loan_q_001",
  "principal": 50000000,
  "termMonths": 12,
  "annualInterestRate": "16.3",
  "interestRateUnit": "percent_per_year",
  "regionGroup": "KV01",
  "incomeBracket": "10M_TO_UNDER_15M",
  "installmentMethod": "fixed_total_reducing_balance",
  "estimatedMonthlyPayment": null,
  "estimatedTotalInterest": null,
  "estimatedTotalPayment": null,
  "validUntil": "2026-10-12T09:15:00+07:00",
  "isBindingOffer": false
}
```

- Thu nhập, khu vực phải lấy từ hồ sơ khách hàng **đã được kiểm chứng**. Ví dụ dùng mức **16,3%/năm** tương ứng KV01 và thu nhập từ 10 đến dưới 15 triệu ở chính sách V2.
- Client không được gửi `regionGroup`, `incomeBracket` hay `annualInterestRate` để tự chọn mức lãi.
- `null` ở các trường dự tính nghĩa là **công thức, quy tắc làm tròn/chốt ngày chưa thống nhất**, không phải response cuối cùng.
- Quote chỉ để tham khảo, chưa phải quyết định duyệt vay.

**Validate / error responses**

| HTTP | code | Khi nào |
|------|------|---------|
| 400 | `INVALID_AMOUNT` | Số tiền không dương hoặc không đúng định dạng |
| 400 | `UNSUPPORTED_TERM` | Kỳ hạn chưa được ngân hàng hỗ trợ |
| 403 | `KYC_REQUIRED` | Chưa hoàn thành định danh bắt buộc |
| 422 | `INCOME_NOT_VERIFIED` | Chưa có dữ liệu thu nhập được xác nhận để xác định biểu lãi |
| 404 | `LOAN_PACKAGE_NOT_FOUND` | Gói vay không tồn tại/đã ngừng áp dụng |

### 2.3 Tạo hồ sơ vay nháp

`POST /api/loan-applications`

**Headers:** `Idempotency-Key: <uuid v4>`

**Request**

```json
{
  "packageId": "loan_pkg_personal",
  "requestedAmount": 50000000,
  "termMonths": 12,
  "disbursementAccountId": "acc_001"
}
```

**Response — 201**

```json
{
  "applicationId": "loan_app_001",
  "applicationNo": "LA20261012001",
  "status": "draft",
  "requestedAmount": 50000000,
  "termMonths": 12,
  "requiredDocumentTypes": [
    "IDENTITY_FRONT",
    "IDENTITY_BACK",
    "VNEID_PERSONAL",
    "VNEID_RESIDENCE",
    "VSSID_PARTICIPATION",
    "VSSID_DETAIL",
    "SALARY_STATEMENT_3_MONTHS"
  ],
  "createdAt": "2026-10-12T09:00:00+07:00"
}
```

- Khách hàng phải là chủ tài khoản nhận giải ngân; theo quy trình nghiệp vụ, khoản vay chuyển về **tài khoản nhận lương chính chủ** (BE cần bước xác minh).
- Danh sách `requiredDocumentTypes` là mapping **đề xuất** từ danh mục tài liệu nghiệp vụ; quy trình tải lên và cách xác minh VNeID/VssID cần review.

### 2.4 Tải chứng từ

`POST /api/loan-applications/{id}/documents` — `multipart/form-data`

Các phần gửi lên: `documentType` (một giá trị trong `requiredDocumentTypes`) và `file` (ảnh/PDF).

**Response — 201**

```json
{
  "documentId": "loan_doc_001",
  "documentType": "SALARY_STATEMENT_3_MONTHS",
  "fileName": "sao-ke.pdf",
  "mimeType": "application/pdf",
  "sizeBytes": 245800,
  "verificationStatus": "pending",
  "uploadedAt": "2026-10-12T09:02:00+07:00"
}
```

- BE kiểm tra MIME, kích thước, định dạng thực tế và quyền sở hữu hồ sơ; lưu file ở private storage.
- Không trả `storage_key`, ảnh định danh hoặc dữ liệu cá nhân không cần thiết về client.
- `multipart/form-data` là **phương án API đề xuất**; nếu repo đã có RustFS/presigned upload thì dùng luồng hiện tại thay vì tự tạo thêm hệ thống upload.

**Error responses**: `404 APPLICATION_NOT_FOUND`; `409 APPLICATION_NOT_EDITABLE`; `400 UNSUPPORTED_FILE_TYPE`; `413 FILE_TOO_LARGE` (giới hạn kích thước là OQ).

### 2.5 Gửi hồ sơ để thẩm định

`POST /api/loan-applications/{id}/submit` — body rỗng, có `Idempotency-Key`.

**Response — 200**

```json
{
  "applicationId": "loan_app_001",
  "status": "submitted",
  "submittedAt": "2026-10-12T09:05:00+07:00",
  "message": "Hồ sơ đã được tiếp nhận và đang chờ thẩm định."
}
```

BE kiểm tra lại thông tin định danh, tính đầy đủ chứng từ và trạng thái hồ sơ. Thẩm định có thể chạy tự động hoặc chuyển xử lý thủ công; không tự động trả `approved` chỉ vì client gửi hồ sơ thành công.

**Error responses**: `422 DOCUMENTS_INCOMPLETE`; `422 KYC_NOT_VERIFIED`; `422 INVALID_DISBURSEMENT_ACCOUNT`; `409 APPLICATION_ALREADY_SUBMITTED`.

### 2.6 Danh sách hồ sơ

`GET /api/loan-applications?status=all&limit=20&cursor=`

**Response — 200**

```json
{
  "items": [
    {
      "applicationId": "loan_app_001",
      "applicationNo": "LA20261012001",
      "requestedAmount": 50000000,
      "status": "under_review",
      "createdAt": "2026-10-12T09:00:00+07:00"
    }
  ],
  "nextCursor": null
}
```

Tối đa 50 bản ghi/trang, chỉ trả hồ sơ của user đăng nhập; sắp xếp mới nhất trước.

### 2.7 Chi tiết và kết quả hồ sơ

`GET /api/loan-applications/{id}`

**Response — 200**

```json
{
  "applicationId": "loan_app_001",
  "status": "approved",
  "requestedAmount": 50000000,
  "assessment": {
    "result": "approved",
    "method": "manual_or_automatic",
    "assessedAt": "2026-10-13T10:00:00+07:00"
  },
  "offer": {
    "approvedAmount": 50000000,
    "annualInterestRate": "16.3",
    "termMonths": 12,
    "expiresAt": null
  },
  "canAcceptOffer": true
}
```

- `offer` chỉ hiện khi đã có kết quả phê duyệt. Với `rejected`, BE trả trạng thái và lý do được phép công khai (nếu có).
- `assessment.method` là placeholder, sẽ chốt enum trước triển khai.

**Error responses**: `404 APPLICATION_NOT_FOUND` nếu không tồn tại **hoặc không thuộc** khách hàng.

### 2.8 Chấp nhận đề nghị vay / ký hợp đồng

`POST /api/loan-applications/{id}/accept-offer` — có `Idempotency-Key`.

**Request (dự kiến)**

```json
{
  "acceptTerms": true
}
```

**Response — 202**

```json
{
  "applicationId": "loan_app_001",
  "status": "awaiting_contract_verification",
  "nextAction": "VERIFY_CONTRACT"
}
```

**Chưa chốt:** UC-S06 cho phép xác nhận hợp đồng bằng OTP hoặc chữ ký điện tử. API xác thực hợp đồng và thời điểm giải ngân phải thiết kế tiếp sau khi nhóm chọn một phương án. **Không giải ngân chỉ vì nhận `acceptTerms: true`**.

**Error responses**: `409 OFFER_NOT_APPROVED`; `409 OFFER_ALREADY_ACCEPTED`; `422 OFFER_EXPIRED` (nếu có thời hạn); `400 TERMS_NOT_ACCEPTED`.

### 2.9 Danh sách khoản vay

`GET /api/loans?status=active&limit=20&cursor=`

**Response — 200**

```json
{
  "items": [
    {
      "loanId": "loan_001",
      "loanNo": "LN20261014001",
      "principal": 50000000,
      "outstandingPrincipal": 50000000,
      "annualInterestRate": "16.3",
      "termMonths": 12,
      "status": "active"
    }
  ],
  "nextCursor": null
}
```

### 2.10 Chi tiết khoản vay

`GET /api/loans/{id}`

**Response — 200**

```json
{
  "loanId": "loan_001",
  "applicationId": "loan_app_001",
  "loanNo": "LN20261014001",
  "principal": 50000000,
  "outstandingPrincipal": 50000000,
  "annualInterestRate": "16.3",
  "termMonths": 12,
  "startDate": "2026-10-14",
  "maturityDate": "2027-10-14",
  "status": "active"
}
```

Các API lịch trả nợ và thanh toán nằm trong **`loan-repayment-spec.md`**; không định nghĩa lại ở đây.

---

## 3. Schema và liên hệ MongoDB

| API field | Collection/trường liên quan | Ghi chú |
|-----------|-----------------------------|---------|
| `applicationId`, `status` | `loan_applications._id`, `status` | Hồ sơ vay |
| `packageId` | `loan_packages._id` | Danh mục gói |
| `requestedAmount`, `termMonths` | `loan_applications.request` | Số tiền/kỳ hạn đề nghị |
| `annualInterestRate` khi duyệt | `loan_applications.offer.interest_rate` | Mức phê duyệt |
| `loanId`, `outstandingPrincipal` | `loans._id`, `loans.outstanding_principal` | Sau giải ngân |
| `annualInterestRate` trong hợp đồng | `loans.interest_rate` | Snapshot tại thời điểm ký |
| Chứng từ | `loan_applications.documents[]` | Chỉ metadata + `storage_key` |
| Bảng lãi suất hiệu lực | `interest_rate_policies` **đề xuất** | Đã ghi trong Word V2, **không sửa JSON** |

ID dạng `loan_app_001` chỉ để minh họa; DB vẫn dùng ObjectId theo thiết kế, API có thể serial hóa thành string. Không cho client tự chỉ định điểm tín dụng, mức lãi hay kết quả thẩm định.

---

## 4. Quy tắc nghiệp vụ

1. **Nhóm thu nhập:** dưới 7 triệu; từ 7 đến dưới 10; từ 10 đến dưới 15; từ 15 đến dưới 20; từ 20 đến dưới 30; từ 30 triệu trở lên.
2. **Nhóm khu vực:** `KV01` = TP.HCM, Cần Thơ, Hà Nội; `KV02` = Huế, Đà Nẵng, Hải Phòng; `KV03` = các tỉnh/thành phố còn lại.
3. **Lãi suất vay V2 kỳ hạn 12 tháng (%/năm):**

   | Thu nhập/tháng | KV01 | KV02 | KV03 |
   |----------------|-----:|-----:|-----:|
   | Dưới 7 triệu | 17,5 | 17,7 | 17,9 |
   | 7 – dưới 10 triệu | 16,9 | 17,1 | 17,3 |
   | 10 – dưới 15 triệu | 16,3 | 16,5 | 16,7 |
   | 15 – dưới 20 triệu | 15,7 | 15,9 | 16,1 |
   | 20 – dưới 30 triệu | 15,0 | 15,2 | 15,4 |
   | Từ 30 triệu | 14,4 | 14,6 | 14,8 |

4. Không tự duyệt chỉ dựa vào thu nhập. Cần thẩm định giấy tờ định danh, sao kê lương 3 tháng, VssID, DTI, nghĩa vụ nợ và các điều kiện ngân hàng.
5. Không nhận `annualInterestRate`, `creditScore`, `approvedAmount` từ client làm giá trị tin cậy.
6. Giải ngân phải gắn với hợp đồng đã xác nhận hợp lệ và tài khoản nhận lương chính chủ được xác minh; cập nhật tài khoản và phát sinh giao dịch không được nhân đôi do retry.
7. Mỗi thay đổi quan trọng nên có event/audit theo kiến trúc dự án, ví dụ `LoanApplicationSubmitted`, `LoanApplicationApproved`/`Rejected`, `LoanContractAccepted`, `LoanDisbursed`. Tên event **đề xuất**, cần thống nhất với convention chung và không chứa ảnh/CCCD thô trong payload.
8. Nghiệp vụ vay thêm theo UC-06 có thể tái sử dụng quy trình, nhưng phải tính tổng nghĩa vụ trả nợ. Việc bật tính năng vay thêm thuộc phạm vi mở rộng, chưa mặc định có trong bản đầu.

---

## 5. Mapping client ↔ API (dự kiến)

| Hành động trên app | API |
|-------------------|-----|
| Mở màn vay, xem kỳ hạn | `GET /api/loans/packages` |
| Nhập số tiền, xem tạm tính | `POST /api/loans/quotes` |
| Tạo hồ sơ | `POST /api/loan-applications` |
| Tải CCCD / VNeID / VssID / sao kê | `POST /api/loan-applications/{id}/documents` |
| Bấm "Gửi hồ sơ" | `POST /api/loan-applications/{id}/submit` |
| Xem tiến độ, quyết định thẩm định | `GET /api/loan-applications/{id}` |
| Đồng ý đề nghị vay | `POST /api/loan-applications/{id}/accept-offer` (chưa hoàn tất xác thực hợp đồng) |
| Xem khoản vay đang có | `GET /api/loans`, `GET /api/loans/{id}` |

---

## 6. Open Questions cho nhóm / BE

| # | Câu hỏi chưa thống nhất | Ảnh hưởng |
|---|-------------------------|-----------|
| OQ-01 | Hạn mức vay tối thiểu/tối đa cho một hồ sơ là bao nhiêu? | Validation 2.2–2.3 |
| OQ-02 | Chỉ mở kỳ hạn **12 tháng** trong MVP, hay có 24/36 tháng? Nếu mở rộng thì lấy bảng lãi nào? | Packages, quotes, schedule |
| OQ-03 | Thu nhập bình quân và khu vực lấy từ trường dữ liệu nào, cập nhật/xác minh khi nào? | Chọn lãi suất chính xác |
| OQ-04 | DTI tối đa, điểm tín dụng tối thiểu, chính sách từ chối hồ sơ là gì? | Thẩm định tự động |
| OQ-05 | VNeID/VssID được tích hợp thật hay chỉ kiểm tra chứng từ mô phỏng? | Scope, API, bảo mật |
| OQ-06 | Dùng `multipart` hay cơ chế RustFS/presigned upload đang có? Giới hạn file? | API 2.4 |
| OQ-07 | Xác nhận hợp đồng bằng OTP email hay chữ ký điện tử? Thời điểm trừ/cộng tiền và hoàn tất giải ngân? | API 2.8, bảo mật |
| OQ-08 | Công thức trả góp đều, quy tắc ngày, làm tròn và lãi năm tính 365 ngày hay 12 tháng? | Quote và schedule |
| OQ-09 | Có màn hình nhân viên duyệt thủ công trong cùng sprint không? | API admin riêng |
| OQ-10 | Khi EventStoreDB/Mongo projection trễ hoặc thao tác giải ngân lỗi giữa chừng, trạng thái hiển thị và retry ra sao? | Tính nhất quán / idempotency |

---

*Phiên bản: 0.1 — đặc tả độc lập cho Vay vốn cá nhân.*  
*Ngày soạn: 2026-10-12.*  
*Trạng thái: Draft — chưa code, chờ review và chốt Open Questions.*
