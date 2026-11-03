# Spec: Quản lý gói vay và gói tiết kiệm phía admin

| Thông tin | Chi tiết |
|-----------|----------|
| **Phạm vi** | Danh mục gói vay/tiết kiệm trên admin desktop, màn tra cứu hợp đồng, và đối soát luồng user/admin |
| **Trạng thái** | v0.2, ngày 2026-11-03. Đã triển khai P0 đến P2 (chưa commit); P3 chưa làm trừ F-15. Xem mục 12 |
| **Cơ sở đối chiếu** | Commit `38063cb` cộng thay đổi chưa commit trên `feat/mobile-card-ui`; số dòng tính theo working tree |
| **Cách đối soát** | Đọc code tĩnh. Chưa chạy app hay e2e, nên các lỗi ở mục 7 chưa được tái hiện khi chạy thật |
| **Thay thế** | Mục 10 của `admin-spec.md` (mô hình phí, phạt, một lãi suất cố định không khớp domain hiện tại) |

## 1. Hiện trạng

| Năng lực | Gói vay | Gói tiết kiệm |
|----------|---------|---------------|
| Màn admin | Route `#/admin/loan-packages` có trong menu nhưng chỉ là placeholder (`client/desktop/src/app/App.tsx:111`) | Không có route, không có menu |
| API admin | Chỉ có `POST /api/admin/loans/catalog/seed`, dùng cho dev | Không có |
| Lưu trữ | Mongo `loan_packages`, unique theo `package_code` | Mongo `savings_packages`, `savings_terms` |
| Khởi tạo | Chỉ seed dev; không có migration nên môi trường production không có gói | Tự seed khi API khởi động |
| Mã gói | Hardcode `PERSONAL_UNSECURED_SALARY` (`loan_package_catalog.go:148`, `loan_handler.go:619`) | Hardcode `savings_standard` (`quote.go:15`) |
| Kỳ hạn | Hardcode 6/12/24/36/48/60 ở 3 nơi | Hardcode 7 kỳ hạn trong `domain/savings/policy.go` |
| Lãi suất | Ma trận thu nhập × khu vực × kỳ hạn trong code (`interest_rate.go`), không nằm trong gói | Bảng `approvedTermRates` trong code; catalog khác bảng này thì API từ chối |

Sửa Mongo trực tiếp không đổi được lãi suất tiết kiệm. Vì vậy cần đổi domain trước khi làm màn quản lý (mục 5).

## 2. Mục tiêu và ngoài phạm vi

**Mục tiêu**

1. Admin xem được danh mục gói, lịch sử phiên bản và số liệu sử dụng.
2. Admin sửa, phát hành và ngừng bán gói mà không deploy, không ảnh hưởng hợp đồng đang chạy.
3. Admin tra cứu được hợp đồng vay, sổ tiết kiệm, lịch trả và khoản quá hạn của mọi khách.
4. Mọi thay đổi gói có vết: ai đổi, đổi gì, lúc nào.

**Ngoài phạm vi**

- Phí, phạt, các phương thức tính lãi khác, chứng từ cấu hình theo gói.
- Sửa ma trận lãi suất vay trên UI. Ma trận này nằm trong chính sách thẩm định `loan-policy-v1`; admin chỉ xem và mô phỏng.
- Hẹn giờ hiệu lực. Bản đầu có hiệu lực ngay khi phát hành.
- Phân quyền chi tiết. Hệ thống hiện chỉ có role `admin`; xem câu hỏi Q3.

## 3. Mô hình dữ liệu

Gói là cấu hình, không phải sổ cái. EventStoreDB vẫn là nguồn sự thật của tiền và hợp đồng. Catalog ở lại Mongo theo kiểu phiên bản bất biến.

### 3.1 Trạng thái phiên bản

```text
DRAFT ──publish──> ACTIVE ──có bản mới phát hành──> SUPERSEDED
  │                   │
xóa được              └──retire──> RETIRED
```

- Mỗi mã gói có tối đa một `ACTIVE` và một `DRAFT`.
- `ACTIVE`, `SUPERSEDED`, `RETIRED` không sửa được, không xóa được.
- Muốn bán lại gói đã `RETIRED`: tạo bản nháp mới rồi phát hành.

### 3.2 Schema

Trường chung của cả hai collection:

```text
package_code, version, status, name, description,
created_by, created_at, published_by, published_at, retired_by, retired_at, retire_reason
```

| Collection | Trường riêng | Ràng buộc khi phát hành |
|------------|--------------|-------------------------|
| `loan_packages` | `min_amount`, `max_amount`, `term_options[{months}]` | `0 < min ≤ max`; kỳ hạn là tập con không rỗng của 6/12/24/36/48/60 (kỳ hạn mà `CalculateAnnualLoanRateBps` tính được) |
| `savings_packages` | `currency`, `min_deposit`, `term_options[{months, interest_rate}]` | `min_deposit ≥ 1.000.000`; kỳ hạn là tập con không rỗng của 3/6/9/12/24/36/60; lãi `0 < r ≤ 100`, tối đa 2 chữ số thập phân |

Lý do của các giới hạn:

- `min_deposit` không được thấp hơn `MinimumDepositVND`, vì `SavingsMovement.Validate` và `CalculateMaturityInterest` dùng hằng số này để kiểm tra sự kiện cũ. Hằng số phải giữ nguyên làm sàn, nếu không replay sẽ hỏng.
- Trần 100% khớp giới hạn `AnnualRateBps ≤ 10_000` trong `Validate`.
- Bỏ một kỳ hạn là an toàn với sổ cũ, vì replay đã dùng `MaturityForStoredContract`.

Index:

- `{package_code: 1, version: 1}` unique.
- `{package_code: 1}` unique một phần với `status: "ACTIVE"`, và tương tự với `status: "DRAFT"`.

Bản ghi audit nằm ở collection mới `product_catalog_audit`: `{package_type, package_code, version, action, actor_id, at, before, after}`. Audit được ghi cùng transaction với thay đổi gói (replica set `rs0` đã có sẵn).

### 3.3 Migration

Việc chuyển đổi chia làm hai phần:

- **Khi API khởi động** (`EnsureCatalogIndexes`, chạy lại an toàn): gán `version: 1` và chuẩn hóa `status` cho bản ghi cũ, bỏ unique index `package_code_1`, tạo các index mới. Phần này nằm trong code vì collection có thể chưa tồn tại khi migration chạy, và lệnh `dropIndexes` sẽ lỗi trên collection chưa có.
- **Migration `000017`**: seed `PERSONAL_UNSECURED_SALARY` bằng `$setOnInsert` để production có gói vay. Mức hạn mức theo Q4.

Thứ tự triển khai: dừng toàn bộ API và projector cũ, chạy migration, rồi bật bản mới. API cũ khởi động lại sẽ tạo lại unique index trên `package_code`, và lệnh này thất bại nếu đã có nhiều phiên bản. Projector phải lên cùng lúc: bản cũ từ chối sự kiện trả nợ quá hạn và sự kiện `LoanApplicationReviewRejected`.

## 4. Ảnh hưởng của thay đổi gói

| Đối tượng | Phát hành bản mới | Ngừng bán |
|-----------|-------------------|-----------|
| Sổ tiết kiệm đã mở | Không đổi; lãi là snapshot trong sự kiện | Không đổi; vẫn rút được |
| Quote mở sổ còn hạn (5 phút) | Vẫn dùng lãi đã báo | Bị từ chối, trả `SAVINGS_PACKAGE_RETIRED` |
| Hồ sơ vay chưa giải ngân | Không đổi; chỉ kiểm tra gói khi tạo hồ sơ | Không đổi; hồ sơ đã tạo đi tiếp |
| Tạo hồ sơ hoặc quote mới | Dùng bản mới | Bị từ chối, `GET .../packages` trả danh sách rỗng |
| Điều khoản `savings_terms` | Độc lập; khách phải đồng ý bản đang hiệu lực khi mở sổ | Không liên quan |

Thêm trường cộng thêm `packageVersion` vào `LoanApplicationCreated`, `SavingsOpened` và savings quote. Sự kiện cũ không có trường này được hiểu là phiên bản không xác định. Quote cũ trong Redis hết hạn sau 5 phút nên không cần chuyển đổi.

## 5. Thay đổi server cần có

**Tiết kiệm**

- Lãi và kỳ hạn lấy từ catalog. `approvedTermRates` chỉ còn là dữ liệu seed.
- `LoadValidQuote`, `OpenSavings` (`bank_savings.go:101`), `NewContract`, `CalculateMaturityAt` không so với bảng cứng nữa. Thay vào đó quote giữ lãi đã chốt và lệnh mở sổ kiểm tra gói chưa bị ngừng bán.
- `ValidateCatalogAt` chỉ kiểm tra tính hợp lệ, không còn so với bảng cứng.
- Gói ngừng bán trả danh sách rỗng thay vì lỗi 503.
- `RateString` đang cắt số thập phân thứ hai (525 bps thành `"5.2"`). Phải giữ đủ 2 chữ số: 525 thành `"5.25"`, 500 vẫn là `"5.0"`.
- Lỗi catalog không được tắt lệnh rút tiền (xem F-03).

**Vay**

- Bỏ hardcode mã gói ở `ListActiveLoanPackages` và `loan_handler.go:619`.
- Gom kỳ hạn hợp lệ vào một hàm domain thay cho 3 nơi lặp lại.

**Tầng code**

- Cổng mới: `application/catalog/commands` và `queries`; adapter ở `infrastructure/mongo`; lỗi adapter được dịch ở `composition`.
- Route đăng ký qua `openapi.Registry`, bọc `RequireAuth` và `RequireCurrentAdmin`.

## 6. API và màn hình admin

Response dùng chuẩn `{status, message, data}` và `{status, title, errors}`.

### 6.1 Danh mục gói

`{type}` là `loan-packages` hoặc `savings-packages`.

| Method | Path | Mục đích |
|--------|------|----------|
| GET | `/api/admin/{type}?status=&q=` | Danh sách theo mã gói, kèm phiên bản đang hiệu lực. Không phân trang vì số mã gói rất nhỏ |
| GET | `/api/admin/{type}/{code}` | Chi tiết, các phiên bản, số liệu sử dụng |
| POST | `/api/admin/{type}/{code}/draft` | Tạo bản nháp từ bản mới nhất |
| PATCH | `/api/admin/{type}/{code}/draft` | Sửa bản nháp |
| DELETE | `/api/admin/{type}/{code}/draft` | Xóa bản nháp |
| POST | `/api/admin/{type}/{code}/publish` | Phát hành |
| POST | `/api/admin/{type}/{code}/retire` | Ngừng bán, bắt buộc có `reason` |
| POST | `/api/admin/loan-simulations` | Mô phỏng lãi và lịch trả bằng domain hiện tại, không lưu |
| POST | `/api/admin/savings-simulations` | Mô phỏng lãi đáo hạn, không lưu |

Điều khoản tiết kiệm: `GET /api/admin/savings-terms`, `PUT` và `DELETE /api/admin/savings-terms/draft`, `POST /api/admin/savings-terms/publish`.

Hàng đợi hồ sơ: `GET /api/loans?all=true&status=submitted,under_review` lọc theo trạng thái. Thêm `GET /api/loans/{id}/evidence` và `POST /api/loans/{id}/reject-review`.

Quy tắc ghi:

- `publish` và `retire` cần `Idempotency-Key` và `expectedVersion`. Lệch phiên bản trả `409 PACKAGE_VERSION_CONFLICT`.
- Lỗi khác: `404 PACKAGE_NOT_FOUND`, `409 DRAFT_ALREADY_EXISTS`, `409 NO_DRAFT`, `422 INVALID_PACKAGE`.
- Số liệu sử dụng đọc từ projection nên có thể trễ. Với sổ tiết kiệm, cần thêm `package_id` index cho `savings_contracts`.

### 6.2 Tra cứu hợp đồng

Tất cả chỉ đọc, dữ liệu từ projection, có cờ `eventualConsistency: true`.

| Method | Path | Bộ lọc |
|--------|------|--------|
| GET | `/api/admin/savings/contracts` | `status`, `packageId`, `customerId`, `q`, `from`, `to`, `cursor`, `limit` |
| GET | `/api/admin/savings/contracts/{id}` | Có lịch sử giao dịch và biên nhận |
| GET | `/api/admin/loans/contracts` | `status`, `packageCode`, `customerId`, `overdue`, `cursor`, `limit` |
| GET | `/api/admin/loans/contracts/{id}` | Có lịch trả và các lần thanh toán |

### 6.3 Màn hình desktop

`protectedRouteFromHash` chỉ khớp chính xác đường dẫn, không hỗ trợ `:code`. Vì vậy dùng bố cục danh sách và chi tiết trên một route, gói đang chọn nằm trong state của trang.

| Route | Trạng thái | Nội dung |
|-------|-----------|----------|
| `#/admin/loan-packages` | Có, đang là placeholder | Danh sách, chi tiết, form bản nháp, mô phỏng. Ma trận lãi xem qua mô phỏng, không chép bảng lãi sang client |
| `#/admin/savings-packages` | Mới | Như trên, form có bảng lãi theo kỳ hạn, tab Điều khoản |
| `#/admin/savings/contracts` | Mới | Tra cứu sổ tiết kiệm. Tìm theo tiền tố mã sổ |
| `#/admin/loans/contracts` | Mới | Tra cứu hợp đồng vay |
| `#/admin/loans/overdue` | Có, chưa vào menu | Danh sách quá hạn, dùng `overdue=true` |

Form phát hành hiển thị bảng so sánh với bản hiện tại và số hồ sơ, sổ đang dùng gói, rồi yêu cầu xác nhận. Khi ngừng bán phải nhập lý do. Khi tiền hoặc lãi đổi, phải hiện rõ rằng hợp đồng cũ không bị ảnh hưởng.

Tab Điều khoản quản lý `savings_terms` theo cùng vòng đời nháp và phát hành. Khi lãi đổi, màn hình nhắc rà soát điều khoản nếu văn bản có nêu con số.

## 7. Kết quả đối soát luồng user/admin

Mức độ: **Cao** là kẹt nghiệp vụ hoặc có thể gây mất khả năng dùng tiền; **Vừa** là sai hoặc thiếu chức năng có đường vòng; **Thấp** là hiển thị.

### 7.1 Lỗi cần sửa

| ID | Mức | Luồng | Vấn đề | Bằng chứng |
|----|-----|-------|--------|------------|
| F-01 | Cao | Vay, chứng từ | Admin từ chối chứng từ thì hồ sơ kẹt. Khách được báo "Vui lòng cập nhật chứng từ" nhưng chỉ tải được khi `draft`. Admin không chấm điểm được vì thiếu xác minh, và chỉ từ chối được hồ sơ sau khi đã chấm điểm ra `manual_review`. Lối thoát duy nhất là đổi chứng từ thành đạt | `domain/loan/application.go:197`, `loan_handler.go:828`, `manual_loan_decision.go:116`, `LoanApplicationsPage.tsx:99,514` |
| F-02 | Cao | Trả nợ | Chỉ trả được đúng ngày đến hạn. Lỡ một ngày là không trả được kỳ đó, và các kỳ sau cũng bị chặn vì phải trả theo thứ tự, nên khoản vay kẹt vĩnh viễn. Không có trạng thái `overdue`, không có job nhắc nợ, và admin không có màn xử lý. App vẫn hiện "Kỳ tới" cho kỳ đã quá hạn | `domain/loan/repayment.go:181,198`, `LoanContractsPage.tsx:315` |
| F-03 | Cao | Tiết kiệm | Đặt gói khác `ACTIVE`, hoặc catalog lệch bảng lãi trong code, thì lần khởi động API sau đó sẽ gán `Commands = nil`. Khi đó quote rút, lệnh rút và biên nhận đều trả 503, tức khách có sổ đáo hạn không rút được tiền. Đây là điều kiện chặn trước khi cho ngừng bán | `cmd/api/main.go:312`, `savings_package_catalog.go:99`, `savings_commands.go:22` |
| F-04 | Cao | Admin vay | Chọn hồ sơ khác không xóa ô lý do. Lý do của hồ sơ trước có thể bị ghi vào quyết định của hồ sơ sau, và sự kiện này không sửa được | `LoanReviewPage.tsx:642-652` |
| F-05 | Cao | Vay, khởi tạo | Production không có đường tạo gói vay: seed chỉ chạy ở dev và không có migration. `GET /api/loans/packages` trả rỗng nên khách không đăng ký được | `loan_products_handler.go:30`, `loan_package_catalog.go:153` |
| F-06 | Vừa | Admin vay | Bấm "Chấm điểm" khi chưa lưu đối soát trả 503 và hiện "Hệ thống đang gặp sự cố", vì `serviceError` coi mọi lỗi chứa chữ `failed` là lỗi hạ tầng. Nút lại không bị khóa trong trường hợp này | `LoanReviewPage.tsx:1035`, `loan_handler.go:524`, `loanadapter/service.go:203` |
| F-07 | Vừa | Admin vay | Chọn hồ sơ hiển thị bản từ projection, không gọi `getLoan`. Khách vừa xác nhận hợp đồng mà projection chưa cập nhật thì nút Giải ngân chưa hiện. Ngược lại, sau mỗi thao tác chỉ phần chi tiết được làm mới, badge trong danh sách vẫn cũ | `LoanReviewPage.tsx:605-608,642-644` |
| F-08 | Vừa | Admin vay | Form đối soát chỉ có một ô lương, gửi thành 3 tháng giống nhau, nên không phản ánh sao kê 3 tháng thật. Một ô tick đặt cả 4 cờ xác minh. Không có API đọc lại evidence, nên sau khi tải lại admin chấm điểm trên dữ liệu không nhìn thấy | `LoanReviewPage.tsx:660-673` |
| F-09 | Vừa | Vay, kết quả | App khách không hiển thị `rejectionReason` của chứng từ, `manualDecisionReason`, và không có panel riêng cho hồ sơ `rejected` | `LoanApplicationsPage.tsx:514-553` (không có tham chiếu nào trong `features/loans` và `pages`) |
| F-10 | Vừa | Vay, đề nghị | Khách từ chối đề nghị thì hồ sơ vẫn `approved` mãi, admin không có thao tác nào và hàng đợi hiện "Đã duyệt". Đề nghị cũng không có hạn: lãi đã chốt có thể được xác nhận và giải ngân sau thời gian dài bất kỳ | `domain/loan/offer.go:61`, `loan_offer.go:33`, `LoanReviewPage.tsx:1100-1134` |
| F-11 | Vừa | Admin | Admin không xem được hợp đồng vay, lịch trả hay sổ tiết kiệm của khách, vì các API đều lọc theo chính người gọi. Không có màn tiết kiệm nào phía admin; Event Explorer cũng không có nhóm tiết kiệm | `loan_contract_handler.go:53-98`, `savings_read.go:99`, `features/audit/model.ts:37` |
| F-12 | Vừa | Tiết kiệm | Worker đáo hạn tạo thông báo và server có `GET /api/savings/notifications`, nhưng cả hai client đều không gọi. Khách không nhận được thông báo đáo hạn | `savings_maturity_worker.go`, `savings_read.go` (handler), không có tham chiếu trong `client/` |
| F-13 | Vừa | Tiết kiệm | Trạng thái `locked` không có đường tạo ra: không sự kiện nào đặt, admin không có lệnh khóa. App vẫn có bộ lọc "Tạm khóa", và `status=locked` không được API danh sách chấp nhận | `domain/savings/contract.go:14`, `queries/contracts.go:241` |
| F-14 | Vừa | Tiết kiệm | Phiên bản điều khoản khách đồng ý không được lưu vào hợp đồng, không có bằng chứng đối chiếu sau này. `savings-terms-mongodb.md` đã ghi nhận việc này | `commands/savings.go:171` |
| F-15 | Vừa | Phân tách nhiệm vụ | Một admin làm được toàn bộ từ xác minh đến duyệt và giải ngân. Không có kiểm tra người xử lý khác chủ hồ sơ; `application()` cho chủ hồ sơ đi tiếp và các lệnh chỉ kiểm tra role | `loan_handler.go:562`, `loanadapter/service.go:105-123` |
| F-16 | Vừa | Tiết kiệm | Sổ đã tất toán mở lại từ danh sách không còn nút xem biên nhận; nút chỉ có ở màn ngay sau khi rút | `SavingsLivePage.tsx:1135` |
| F-17 | Thấp | Admin vay | Hàng đợi "Hồ sơ cần xử lý" gồm cả nháp, đã từ chối, đã giải ngân, và không có bộ lọc | `loan_read_model.go:441`, `loansApi.ts:158` |
| F-18 | Thấp | Tiết kiệm | Thanh tiến độ cố định 35% cho mọi sổ đang gửi | `SavingsLivePage.tsx:653,943` |
| F-19 | Thấp | Tiết kiệm | Ước tính ở bước chọn kỳ hạn tính theo tháng/12, còn server tính ngày thực/365, nên số có thể lệch nhẹ. Màn hình đã ghi rõ là tham khảo | `SavingsLivePage.tsx:329` |
| F-20 | Thấp | Admin | Badge menu cố định "5" và "8" | `AdminShell.tsx:85` |

### 7.2 Phần đã khớp

- Mở sổ, rút tiền, trả nợ đều giữ idempotency và kiểm tra lại trên EventStoreDB; không dựa vào Mongo để quyết định tiền.
- Sổ chuyển `matured` theo thời gian khi đọc, không phụ thuộc worker đáo hạn.
- Lãi tiết kiệm được chốt trong sự kiện; replay không phụ thuộc bảng kỳ hạn hiện tại. Riêng sàn tiền gửi vẫn phụ thuộc hằng số, xem mục 3.2.
- Giải ngân yêu cầu hợp đồng đã xác nhận OTP, dùng lãi đã chốt, và retry không cộng tiền hai lần.
- Điều khoản đổi giữa chừng được xử lý ở cả server (`SAVINGS_TERMS_CHANGED`) và app.

### 7.3 Tài liệu lệch code

- `vayvon.md`: dùng `/api/loan-applications` và chỉ kỳ hạn 12 tháng; code dùng `/api/loans` với 6 kỳ hạn.
- `tranovay.md`: mô tả trạng thái `overdue`; code không có.
- `guitietkiem.md`: request mở sổ thiếu `acceptedTermsVersion`.
- `LOAN_UI_API_NOTES.md` mục 2: nói API không nhận mục đích vay; code đã nhận qua `declared`.
- `server/docs/savings-terms-mongodb.md` và `loan-products-review7.md`: cần cập nhật sau khi có màn quản lý.

## 8. Hướng sửa các lỗi Cao

**F-01.** Không thêm trạng thái mới.

- Cho khách tải thay thế khi hồ sơ `under_review`, chưa chấm điểm, và loại chứng từ đó đang bị từ chối. Sửa cả `AddDocument` và chặn ở `upload`.
- Thêm lệnh `POST /api/loans/{id}/reject-review` để admin từ chối hồ sơ trước khi chấm điểm, bắt buộc có lý do.
- Desktop phải hiển thị chứng từ mới nhất theo loại; hiện `documents.find` lấy bản đầu tiên, sẽ ra bản đã bị từ chối.
- App khách hiện lý do và nút tải lại.
- Thêm test replay cho `LoanDocumentAdded` xuất hiện sau `LoanApplicationSubmitted`.

**F-02.** Cho trả kỳ đã quá hạn, theo mặc định ở Q1.

- `DueInstallment` chấp nhận `today ≥ dueDate`, vẫn chặn trả sớm.
- Trả trạng thái `overdue` khi đọc lịch, tính từ ngày hiện tại, không ghi thêm sự kiện.
- App hiện rõ số kỳ quá hạn. Admin xem qua `#/admin/loans/overdue`.

**F-03.** Tách catalog khỏi lệnh rút.

- `main.go` chỉ tắt mở sổ và quote mở sổ khi catalog lỗi; `Commands` vẫn hoạt động cho rút tiền và biên nhận.
- Thêm test: khởi động với gói `RETIRED`, sổ đáo hạn vẫn rút được.

**F-04.** Đặt lại `reason` trong `selectApplication`. Chuyển trạng thái form theo hồ sơ được chọn và thêm test cho trường hợp này.

**F-05.** Giải quyết bằng migration `000017` (mục 3.3).

## 9. Thứ tự triển khai

| Giai đoạn | Nội dung | Điều kiện hoàn thành |
|-----------|----------|----------------------|
| **P0: sửa luồng** | F-01 đến F-09, F-12 | Có test cho từng lỗi; chạy xong checklist e2e trong `LOAN_UI_API_NOTES.md` |
| **P1: chỉ đọc** | Migration, API và màn xem danh mục, tra cứu hợp đồng, quá hạn (F-11) | Admin xem được gói và hợp đồng của mọi khách |
| **P2: ghi** | Bản nháp, phát hành, ngừng bán, audit; bỏ hardcode lãi tiết kiệm; xử lý F-10, F-13, F-14 | Đạt tiêu chí nghiệm thu mục 10 |
| **P3: gói mới** | Tạo mã gói mới; bỏ hardcode mã gói; app cho chọn nhiều gói; F-15 | Hai gói cùng hoạt động, hợp đồng trỏ đúng gói |

P2 phụ thuộc F-03. Không được bật ngừng bán trước khi F-03 đã xong.

## 10. Tiêu chí nghiệm thu

1. Đổi lãi tiết kiệm trên admin: khách mới thấy lãi mới mà không cần deploy; sổ cũ tất toán đúng lãi cũ.
2. Quote còn hạn được tạo trước khi đổi lãi vẫn mở sổ đúng lãi đã báo.
3. Ngừng bán: không tạo được quote hay sổ mới; app báo tạm ngừng; sổ đáo hạn vẫn rút được, kể cả sau khi khởi động lại API.
4. Hai admin cùng phát hành: một người thành công, người còn lại nhận `409 PACKAGE_VERSION_CONFLICT`.
5. Retry `publish` cùng `Idempotency-Key` không tạo thêm phiên bản.
6. Replay toàn bộ stream `banking` và stream vay sau khi đổi gói cho kết quả số dư và hợp đồng không đổi.
7. Mỗi lần phát hành hoặc ngừng bán có đúng một bản ghi audit; transaction lỗi thì không có cả thay đổi lẫn audit.
8. Tài khoản không phải admin nhận 403 ở mọi API `/api/admin/...` mới, kể cả khi JWT cũ còn hạn nhưng role đã bị hạ.

Test server không cần database thật ở tầng domain và application, theo `make test`. Phần transaction và index dùng Testcontainers với tag `e2e`.

## 11. Câu hỏi cần chốt

Các mặc định dưới đây đã được dùng trong spec. Nếu nghiệp vụ quyết định khác thì sửa mục tương ứng.

| # | Câu hỏi | Mặc định |
|---|---------|----------|
| Q1 | Trả quá hạn có tính phạt không? Ân hạn bao lâu? | Cho trả đủ số tiền của kỳ, không phạt, không ân hạn, cho đến khi có chính sách |
| Q2 | Đề nghị vay có hạn bao lâu? | Chưa thêm ở P0; đề xuất 7 ngày khi làm F-10 |
| Q3 | Có cần tách người tạo và người phát hành gói? | Không bắt buộc ở bản đầu, chỉ ghi vết, vì hệ thống chỉ có một role. Riêng F-15 nên chặn admin xử lý hồ sơ của chính mình |
| Q4 | Hạn mức gói vay khi seed production? | Dùng mức seed dev: 1.000.000 đến 500.000.000 VND, kỳ hạn 6 đến 60 tháng |
| Q5 | Giữ hay bỏ trạng thái `locked` của sổ tiết kiệm? | Bỏ khỏi app ở P0; chỉ thêm lệnh khóa khi có yêu cầu nghiệp vụ, vì việc này cần sự kiện mới |

## 12. Kết quả triển khai

| Giai đoạn | Trạng thái |
|-----------|-----------|
| P0 | Xong: F-01 đến F-09, F-12. Làm sớm thêm F-13 (bỏ bộ lọc `locked` trên app), F-16, F-17, F-18, F-20 |
| P1 | Xong |
| P2 | Xong, gồm F-10 (đề nghị hết hạn sau 7 ngày, chỉ kiểm tra ở lệnh mới nên replay sự kiện cũ vẫn hợp lệ) và F-14 |
| P3 | Chưa làm tạo mã gói mới và cho app chọn nhiều gói: mã gói vẫn hardcode. F-15 đã xong (nhân viên không xử lý được hồ sơ của chính mình) |
| F-19 | Giữ nguyên, ước tính đã ghi rõ là tham khảo |

Điểm khác so với thiết kế ban đầu:

- Sửa bản nháp không có kiểm tra đồng thời; người lưu sau ghi đè. Kiểm tra `expectedVersion` chỉ áp dụng khi phát hành và ngừng bán.
- Sổ `locked`: trạng thái vẫn còn trong domain để đọc dữ liệu cũ, chưa có lệnh khóa (Q5).
- Đề nghị vay hết hạn hoặc bị từ chối không đổi trạng thái hồ sơ; hồ sơ vẫn là `approved` và UI hiện rõ đã kết thúc.

Chưa kiểm chứng: `tests/api/loans/lifecycle.hurl` dừng ở bước giải ngân vì file chưa có các bước đề nghị và OTP mà lệnh giải ngân đã yêu cầu từ trước. Các màn hình mới mới được kiểm bằng test component, chưa xem trên app thật.
