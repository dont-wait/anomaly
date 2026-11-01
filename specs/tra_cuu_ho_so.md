# Tra cứu và duyệt chứng từ hồ sơ vay — Đặc tả

**Trạng thái:** Bản nháp, triển khai sau
**Phạm vi:** Màn "Thẩm định hồ sơ" của admin (`client/desktop`), API vay (`server`), phần hiển thị kết quả trên app khách (`client/mobile`)
**Code liên quan:** `client/desktop/src/features/loans/LoanReviewPage.tsx`, `server/internal/presentation/http/handler/loan/loan_handler.go`, `server/internal/domain/loan/application.go`

## 1. Mục đích

Admin mở một hồ sơ là biết ngay khách đã gửi gì, từng file đã duyệt chưa và khách là ai, rồi xem và duyệt từng chứng từ ngay trên màn hình, không phải tải file về máy.

## 2. Hiện trạng

- Màn thẩm định đã có 3 tab: **Thẩm định**, **Chứng từ**, **Khách hàng**. Giữ nguyên cách chia tab này.
- Tab Chứng từ liệt kê đủ 7 loại giấy tờ, kèm ngày gửi, trạng thái và nút tải về.
- Backend gán `VerificationStatus = "pending"` khi upload và chưa có API đổi trạng thái từng file, nên chứng từ gần như luôn ở "Chờ xác minh".
- Việc xác nhận chứng từ hiện chỉ có một ô tick chung, gửi `documentsVerified` cho cả hồ sơ.
- `GET /api/loans/{applicationId}/documents/{documentId}` trả `Content-Type: application/octet-stream`, nên trình duyệt không tự nhận biết được loại file.
- `ApplicationDocument` có `json:"storageKey"`, nên API vay có thể đang trả key RustFS về client. Cần xác nhận trên response thật.
- `GET /api/accounts/{id}` chỉ trả tên đăng nhập, email, số tài khoản, số dư và trạng thái định danh.
- Danh sách hồ sơ hiện gồm mọi trạng thái, kể cả hồ sơ khách chưa gửi.

## 3. Hành vi sản phẩm

### 3.1 Tab Chứng từ

Mỗi dòng chứng từ khách đã gửi có:

| Thành phần | Hành vi |
| --- | --- |
| Icon con mắt | Mở xem trước ngay trên màn hình |
| Nút tải về | Giữ như hiện tại |
| Trạng thái | Chờ xác minh, Đã xác minh, Không đạt |
| **Duyệt** | Chuyển chứng từ sang Đã xác minh |
| **Từ chối** | Bắt buộc nhập lý do, chuyển sang Không đạt |

- Đầu tab hiện tiến độ, ví dụ "Đã xác minh 3/7 chứng từ".
- Giấy tờ khách chưa gửi vẫn hiện "Khách chưa gửi" và không có nút thao tác.
- Nút Duyệt và Từ chối chỉ bật khi hồ sơ ở trạng thái Đang thẩm định. Ở trạng thái khác chỉ xem được.
- Đã duyệt hoặc từ chối thì vẫn đổi lại được khi hồ sơ còn Đang thẩm định, và mỗi lần đổi đều được ghi audit.
- Chỉ đổi trạng thái sau khi server xác nhận, không cập nhật lạc quan.

### 3.2 Xem trước chứng từ

- Bấm icon con mắt mở hộp thoại trên màn hình hiện tại. Tiêu đề là tên giấy tờ, có nút đóng, và đóng được bằng Esc.
- Ảnh JPEG và PNG hiển thị vừa khung, có phóng to và xoay để đọc giấy tờ chụp nghiêng.
- PDF hiển thị trong hộp thoại nếu WebView hỗ trợ. Nếu không, hiện thông báo kèm nút tải về.
- Có nút chuyển sang chứng từ trước hoặc sau, để duyệt lần lượt không phải đóng mở nhiều lần.
- Nút Duyệt và Từ chối cũng có trong hộp thoại, để admin quyết định ngay khi đang nhìn giấy tờ.
- Tải file qua API có bearer token rồi hiển thị bằng object URL, và thu hồi URL khi đóng. Không dùng link RustFS trực tiếp.
- Có trạng thái đang tải, và trạng thái lỗi kèm nút thử lại.

### 3.3 Tab Khách hàng

- Giữ các thông tin đang có và danh sách hồ sơ khác của khách.
- Bổ sung họ tên, số điện thoại, địa chỉ, ngày sinh và CCCD đã che số, chỉ cho admin.
- Không hiển thị ID nội bộ, storage key hay mật khẩu.

### 3.4 Danh sách hồ sơ

- Mặc định chỉ hiện hồ sơ Chờ duyệt và Đang thẩm định.
- Có bộ lọc để xem các trạng thái còn lại.

### 3.5 Quan hệ với tab Thẩm định

- Bỏ ô tick chung "đã đối chiếu chứng từ". `documentsVerified` do server tính: đúng khi đủ 7 loại và tất cả đều đã xác minh.
- Nút **Lưu kết quả đối soát** khóa cho đến khi đủ điều kiện, và ghi rõ còn chứng từ nào chưa duyệt.
- Tab Thẩm định có một dòng tóm tắt tiến độ chứng từ, bấm vào sẽ chuyển sang tab Chứng từ.

### 3.6 Phía khách hàng

- App khách hiển thị trạng thái thật của từng chứng từ.
- Chứng từ bị từ chối hiện lý do và cho phép tải lại (xem quyết định 1 ở mục 8).

## 4. Hợp đồng API đề xuất

### 4.1 Duyệt hoặc từ chối chứng từ

`POST /api/loans/{applicationId}/documents/{documentId}/verification`

```json
{
  "decision": "verified",
  "reason": "Ảnh mờ, không đọc được số CCCD"
}
```

- `decision` là `verified` hoặc `rejected`. `reason` bắt buộc khi `rejected`, tối đa 500 ký tự.
- Chỉ admin được gọi, và hồ sơ phải ở `under_review`.
- Response trả về hồ sơ đã cập nhật.
- Lưu người duyệt và thời điểm duyệt. Khách chỉ thấy `reason`, không thấy người duyệt.
- Lỗi: 400 dữ liệu sai, 401 chưa đăng nhập, 403 không có quyền, 404 không tìm thấy, 409 sai trạng thái hoặc xung đột phiên bản.

### 4.2 Tải và xem chứng từ

- Giữ `GET /api/loans/{applicationId}/documents/{documentId}`.
- Trả `Content-Type` theo `mimeType` đã lưu, chỉ với danh sách cho phép (PDF, JPEG, PNG), kèm `X-Content-Type-Options: nosniff` và `Cache-Control: no-store`.
- Giữ nguyên kiểm tra quyền: chủ hồ sơ hoặc admin.

### 4.3 Metadata chứng từ

- Thêm `rejectionReason` và `verifiedAt` vào response.
- Không trả `storageKey` và `sha256` ra API. Dùng `json:"-"` hoặc DTO riêng, đồng thời đảm bảo event và projection vẫn lưu được hai trường này.

### 4.4 Thông tin khách hàng

Thêm endpoint chỉ dành cho admin, ví dụ `GET /api/admin/customers/{id}`, thay vì mở rộng `GET /api/accounts/{id}` vốn dùng DTO public. Số CCCD trả về phải đã che. Mỗi lần xem được ghi audit.

## 5. Dữ liệu và luồng ghi

- Thay đổi trạng thái chứng từ đi qua luồng ghi event của hồ sơ vay, kèm kiểm tra phiên bản. Không sửa trực tiếp trên projection.
- Bổ sung event mới và xử lý trong replay và loan projector. Event cũ không có các trường mới vẫn phải đọc được.
- Khách tải lại chứng từ tạo bản ghi mới và giữ bản cũ để làm bằng chứng.
- Mọi quyết định đều ghi audit gồm người duyệt, hồ sơ, chứng từ và kết quả. Không ghi nội dung file.

## 6. Phạm vi triển khai

**Backend:** route qua `openapi.Registry`, command duyệt chứng từ, event và projection, sửa header tải file, ẩn `storageKey`, tính `documentsVerified` ở server, endpoint khách hàng cho admin.

**Desktop:** icon con mắt, hộp thoại xem trước, nút Duyệt và Từ chối, hộp nhập lý do, bổ sung tab Khách hàng, bộ lọc danh sách, bỏ ô tick chung.

**Mobile:** hiển thị trạng thái thật và lý do từ chối, luồng tải lại chứng từ.

## 7. Tiêu chí nghiệm thu

- Giữ nguyên 3 tab hiện có. Màn hình không hiện ID nội bộ, storage key hay thuật ngữ kỹ thuật.
- Bấm icon con mắt xem được ảnh và PDF mà không phải tải về máy, kể cả với token bắt buộc.
- Từ chối không có lý do thì không gửi được, và server cũng từ chối.
- Tài khoản không phải admin không duyệt được, kể cả khi gọi thẳng API.
- Không duyệt được khi hồ sơ không ở `under_review`.
- Hai admin duyệt cùng lúc thì người sau nhận 409, không ghi đè mất dữ liệu.
- Không lưu được kết quả đối soát khi chưa duyệt đủ 7 chứng từ.
- Trạng thái chứng từ vẫn đúng sau khi replay event và dựng lại projection.
- Response API vay không còn `storageKey`.
- Kiểm thử chạy thật với RustFS và API, không chỉ unit test.

## 8. Quyết định cần chốt

1. Chứng từ bị từ chối khi hồ sơ đã gửi thì xử lý thế nào: trả hồ sơ về cho khách bổ sung, hay từ chối cả hồ sơ? Hiện panel nộp chứng từ trên app khách chỉ hiện khi hồ sơ còn `draft`. Đề xuất: thêm trạng thái "Cần bổ sung".
2. Lý do từ chối nhập tự do hay chọn từ danh sách có sẵn? Đề xuất: danh sách có sẵn cộng ô ghi chú, để khách đọc dễ hiểu.
3. Hồ sơ cũ đã được tick chung thì xử lý thế nào: giữ nguyên hay yêu cầu duyệt lại từng file?
4. Có cần tách quyền người duyệt chứng từ và người phê duyệt khoản vay không?
