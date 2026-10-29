# Loan UI ↔ API integration (bản patch để TEST)

**Nguồn:** `AnomalyBank_HuongDan_Swagger_Full_Vay_TraVay_PR115_116_117.docx` (22/10/2026) và code thực tế trong `anomaly(1).rar`.
**Baseline đối chiếu:** `main` commit `dd6d7977981f508e387b6df8fca5c431be24378a` (bản archive được gửi).
**Phạm vi:** bổ sung giao diện và client API trong `client/mobile`; **không sửa backend**.

## Đã gắn

| Nhóm | Endpoint | UI dùng |
|---|---|---|
| PR #115 | `GET /api/loans/packages`, `POST /api/loans/quotes` | `#/loan/register`: danh mục gói ACTIVE, lãi & lịch dự tính do server tính |
| Hồ sơ khách hàng | `POST /api/loans`, `GET /api/loans`, `GET /api/loans/{applicationId}` | Tạo/tra cứu hồ sơ |
| Chứng từ | `POST /api/loans/{applicationId}/documents`, `GET /api/loans/{applicationId}/documents/{documentId}` | Upload 7 file / tải lại có bearer token |
| Hồ sơ | `POST /api/loans/{applicationId}/submit`, `GET /api/loans/{applicationId}/quote` | Nộp và xem quote theo Evidence |
| Admin | `POST .../review`, `POST .../evidence`, `POST .../assess`, `POST .../decision`, `POST .../disburse` | `#/loan/admin` (màn test, server phân quyền admin) |
| PR #116 | `POST .../offer/issue`, `GET .../offer`, `POST .../offer/accept`, `POST .../offer/reject`, `POST .../contract/otp`, `POST .../contract/confirm` | Admin phát hành; khách xem/chọn Offer + OTP |
| PR #117 | `GET /api/loans/contracts`, `GET /api/loans/contracts/detail/{loanId}` | Danh sách/chi tiết hợp đồng |
| Trả nợ | `GET /api/loans/{id}/repayment-schedule`, `POST .../repayments/quote`, `POST .../repayments`, `GET .../payments`, `GET .../payments/{paymentId}` | Lịch trả / quote / confirm / lịch sử / biên nhận |

Tổng cộng 27/27 API module vay & trả vay của phụ lục Word được khai báo trong tầng client API và có lối gọi tương ứng trên giao diện (bao gồm bước admin qua màn test). Login đã có sẵn trong app và không viết lại.

## Route mới

- `#/loan/register`: đăng ký vay, GET packages, POST quote & tạo draft trên server.
- `#/loan/applications`: danh sách hồ sơ.
- `#/loan/applications/:applicationId`: upload / submit / xem trạng thái / Offer / OTP.
- `#/loan/contracts`: danh sách hợp đồng đã giải ngân.
- `#/loan/contracts/:loanId`: hợp đồng, lịch, trả nợ, lịch sử, chi tiết thanh toán.
- `#/loan/admin`: **chỉ dành cho admin trong môi trường TEST**, không nằm trên menu khách hàng, backend kiểm tra role từng action.

## Điểm còn thiếu / cần hoàn thiện về sau

1. **Thiết kế 11 màn:** ảnh thiết kế chỉ là screenshot nhỏ, chưa có file Figma hoặc component gốc. Patch bổ sung *luồng chức năng để test API*, **chưa cam kết giống pixel từng màn M1–M11**; cần hoàn thiện UI/UX sau khi có thiết kế gốc.
2. **Mục đích vay:** UI hiện giữ `purpose/details` tại local storage, nhưng API `POST /api/loans` thật **không nhận các trường đó**. Chưa tự ý gửi trường lạ hoặc thay schema backend; phải bổ sung API/domain nếu cần lưu mục đích.
3. **Tài khoản giải ngân / nguồn trả nợ:** auth profile có `id`, `accountNo` nhưng không cung cấp danh sách `accountId` cho người dùng chọn. Màn test mặc định `user.id`, cho sửa bằng account ID thật; backend kiểm tra chủ sở hữu. Tích hợp account-picker và kiểm tra phân biệt user ID/account ID khi có endpoint đúng.
4. **Báo giá:** thu nhập/khu vực/nợ trong `POST /api/loans/quotes` là **tự khai**, không đưa vào hồ sơ hoặc được xác minh tự động. Admin vẫn phải attest Evidence trước khi assess. Đây là cách backend hiện thiết kế.
5. **Phê duyệt/giải ngân:** admin là một thao tác tách biệt và không được chuyển thành hành động của customer. Màn admin hiện là UI kiểm thử cơ bản, chưa thay thế dashboard nghiệp vụ nhân viên đầy đủ.
6. **Offer OTP:** cần `LOAN_OFFER_WRITES_ENABLED=true` **chỉ sau khi** hoàn tất rollout theo `server/docs/loan-offer-contract.md`; cần Redis, SMTP/email đã xác minh và Loan Projector mới. Không bypass OTP.
7. **Repayment:** API chỉ cho quote/trả **đủ một kỳ đúng ngày đến hạn**. Không tạo chức năng trả trước/trễ/trả nhiều kỳ trên client; muốn test trả thành công cần fixture được nhóm phê duyệt tới đúng hạn.
8. **Eventual consistency:** sau submit/offer/disburse/repay, read model Mongo có thể cập nhật trễ; UI có nút tải lại, không tái giải ngân hoặc trả nợ bằng key khác vì chưa thấy projection.
9. **Idempotency:** khi tạo hồ sơ/trả nợ trong cùng màn, client giữ key để retry cùng request nếu lỗi. Khi reload app mất key chưa có quy trình tra cứu giao dịch treo đầy đủ; nên bổ sung trạng thái pending/lookup trước khi người dùng thử lại.
10. **Tính thời gian và bảo mật:** quote trả nợ 5 phút; client kiểm tra hạn trước xác nhận, server vẫn quyết định. Không lưu JWT/OTP vào file test, không dùng CCCD/PDF thật.

## Chuẩn bị backend

- `MongoDB` có gói `PERSONAL_UNSECURED_SALARY` ACTIVE, `minAmount/maxAmount/termMonths` hợp lệ.
- `EventStoreDB`, `Redis`, `RustFS` và `server/cmd/loan-projector` cùng version; KYC hoàn tất; có tài khoản nhân viên admin đã kích hoạt.
- `Go API` ở `http://localhost:8080` (hoặc cấu hình `VITE_API_ENDPOINT`).
- Cấu hình SMTP/email test và cờ Offer đúng quy trình **không bật nếu chưa sẵn sàng**.

## Checklist test end-to-end (tài khoản test)

1. **Customer** đăng nhập → vào `#/loan/register` → GET packages hiển thị gói ACTIVE. Nếu gói rỗng, dừng để kiểm tra Mongo, không hardcode.
2. Nhập amount trong hạn mức, term trong danh mục, income/region/debt → nhấn **Tính lãi và lịch trả qua API**, đối chiếu `POST /api/loans/quotes`; không được còn giá lãi `13.8%` hardcode.
3. Kiểm tra account ID đúng chủ → **Tạo hồ sơ vay trên hệ thống** → `POST /api/loans` (201), chuyển sang hồ sơ mới. Retry phải dùng cùng `Idempotency-Key` nếu thao tác chưa rõ kết quả.
4. Upload đủ **7 loại chứng từ** PDF/PNG/JPEG tối đa 10 MiB, xem lại/download từng file → **Gửi hồ sơ thẩm định** (`submitted`). Thiếu chứng từ phải bị chặn.
5. **Admin** đăng nhập và mở `#/loan/admin` → list all → review → attest Evidence test (đủ 4 cờ verified) → assess → nếu `manual_review` thì quyết định approve → issue Offer. Không dùng giấy tờ/thông tin thật.
6. **Customer** mở lại hồ sơ, tải Offer → accept hoặc reject (reject kiểm trên hồ sơ khác) → request OTP qua email → nhập OTP + tick điều khoản → confirm `confirmed`.
7. **Admin** kiểm tra Offer `confirmed` → disburse đúng 1 lần. **Customer** xem hợp đồng qua `#/loan/contracts`; nếu 503 `CONTRACT_PROJECTION_PENDING` kiểm tra projector, không giải ngân lại.
8. Vào chi tiết hợp đồng, đối chiếu `GET .../repayment-schedule`. Nếu chưa đúng ngày đến hạn, `POST .../repayments/quote` trả `422 INSTALLMENT_NOT_DUE` là **PASS** cho case chặn trả sớm.
9. Với fixture test hợp lệ **đến đúng ngày hạn**, quote 200 → confirm bằng cùng sourceAccountId, giữ idempotency key → GET payments → GET chi tiết payment → tải lại hợp đồng/dư nợ.
10. Test negative case: 401 chưa auth; 403 admin role; 409 state conflict; 410 quote expired; 422 invalid installment/order; double submit/repay; cursor thay đổi; lưu HTTP thật trong checklist Word.
