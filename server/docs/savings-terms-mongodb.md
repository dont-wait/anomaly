# Nội dung điều kiện tiết kiệm trong MongoDB

API `GET /api/savings/terms` lấy dữ liệu **trực tiếp từ MongoDB**, collection
`anomaly.savings_terms`. Không có nội dung điều khoản dự phòng trong mã Go.
Frontend hiện tại giữ nguyên response `{version,title,introduction,sections}`.

## Triển khai trên môi trường phát triển

1. Sao lưu MongoDB trước khi chạy migration. Đảm bảo dùng đúng database trong `server/.env`.
2. Kiểm tra có migration số 16 từ nhánh khác hay chưa (`go run ./cmd/migrate status`).
3. **Nếu migration hiện tại đã là 15**, chạy `go run ./cmd/migrate up`, rồi `status`.
   Migration 16 thêm unique index và tạo nội dung `savings-standard-v1` **một lần**
   bằng `$setOnInsert`, không ghi đè tài liệu đã tồn tại.
   **Nếu hiện tại còn ở 13 hoặc thấp hơn**, đừng chạy `up` chỉ để lấy điều khoản:
   migration 14 có bước xóa dữ liệu risk legacy. Cần kiểm tra các migration giữa kỳ
   và sao lưu trước hoặc dùng phương án Compass dưới đây chỉ dành cho local.
4. **Nạp dữ liệu local khi chưa chạy được chuỗi migration:** sử dụng mongosh bên
   trong Docker, không dùng `load()` của MongoDB Compass (Compass không hỗ trợ
   hàm `load`). Từ PowerShell ở thư mục repository:

   ```powershell
   docker cp server/scripts/mongosh/seed-savings-terms.js anomaly-mongo:/tmp/seed-savings-terms.js
   docker exec anomaly-mongo mongosh --quiet "mongodb://localhost:27017/anomaly" /tmp/seed-savings-terms.js
   ```

   Khi database yêu cầu xác thực, phải dùng tài khoản MongoDB được cấu hình
   cho container và nhập mật khẩu an toàn. Script chỉ upsert theo `_id`, kiểm
   tra trùng bản `ACTIVE` trước khi tạo unique index và không đụng dữ liệu tiền.
   Nếu sau đó chạy migration 16, việc seed sẽ bỏ qua văn bản đã có.
5. Khởi động lại Go API, đăng nhập khách hàng, gọi `GET /api/savings/terms`.
   Nếu chưa triển khai DB hoặc dữ liệu không hợp lệ, API trả `503 SAVINGS_TERMS_UNAVAILABLE`.

Trong MongoDB Compass → Shell:

```javascript
use("anomaly");
db.savings_terms.find(
  { product_code: "savings_standard" },
  { _id: 1, version: 1, status: 1, title: 1, sections: 1 }
).pretty();
```

## Sửa điều khoản sau này

- **Không** chỉnh văn bản trong file Go. Dùng tab Điều khoản ở `#/admin/savings-packages`
  (xem `specs/quan-ly-goi-admin.md`): lưu bản nháp rồi phát hành. Lệnh phát hành hạ
  bản cũ xuống `INACTIVE` và bật bản mới trong cùng transaction.
- Không xóa bản cũ: giữ để phục vụ đối chiếu nội dung từng phiên bản.
- Kỳ hạn, lãi suất và mức gửi tối thiểu được đổi qua phiên bản gói, không qua collection
  này. Công thức tính lãi và thời điểm đáo hạn do Savings Domain quyết định. Chỉ sửa
  nội dung mô tả cho thống nhất với gói đang bán.
- Hợp đồng mở từ bản này lưu `AcceptedTermsVersion` trong sự kiện `SavingsOpened`.
  Hợp đồng cũ không có trường này.

Migration `down` xóa riêng document seed theo `_id` và bỏ unique index,
**không xóa** collection hoặc hợp đồng/số dư. Chỉ rollback có kiểm soát.

### Kiến trúc nguồn dữ liệu

`savings_terms` là danh mục **nội dung sản phẩm** do nghiệp vụ công bố, không phải sổ cái tài chính. MongoDB lưu phiên bản điều kiện được ban hành; EventStoreDB vẫn là nguồn sự thật cho các giao dịch và số dư. Nội dung được phát hành qua migration/script có kiểm soát và chỉ một bản `ACTIVE` cho mỗi sản phẩm. Trước khi triển khai thật, đội vận hành cần quyết định quy trình phê duyệt và lưu vết bản phát hành sản phẩm.

Migration down xóa duy nhất document seed `savings-standard-v1` thay vì đặt `INACTIVE`, để rollback rồi re-up có thể tạo lại điều khoản. Không tự chạy migration 14–16 trên database có dữ liệu đang sử dụng mà chưa rà soát migration 14.
