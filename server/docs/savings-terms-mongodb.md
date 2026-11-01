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
4. **Cấp dữ liệu trực tiếp trong MongoDB Compass ở môi trường local** (không cần
   chạy các migration khác): mở Compass → MONGOSH rồi thực thi:

   ```javascript
   load("E:/KhoaLuan/Project/anomaly/server/scripts/mongosh/seed-savings-terms.js")
   ```

   Script tạo index và `upsert` theo `_id`, dùng `$setOnInsert` nên chạy lại không
   ghi đè các điều khoản đã tồn tại. Script không đụng tới các collection tài chính.
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

- **Không** chỉnh văn bản trong file Go. Chuẩn bị văn bản đã được nhóm duyệt,
  tạo tài liệu mới trong `savings_terms` với `_id` và `version` mới, ban đầu `INACTIVE`.
- Kiểm tra nội dung trước khi xuất bản. Chỉ đổi `ACTIVE` theo quy trình phát hành
  kiểm soát, ưu tiên transaction MongoDB trên replica set (hạ phiên bản cũ
  xuống `INACTIVE` và bật phiên bản mới trong cùng transaction).
- Không xóa bản cũ: giữ để phục vụ đối chiếu nội dung từng phiên bản.
- Các thay đổi về tiền, kỳ hạn, lãi suất, tính lãi, hoặc thời điểm đáo hạn **không**
  được thực hiện bằng cách sửa collection này. Chúng do Savings Domain / quote
  và hợp đồng quyết định; chỉ sửa nội dung mô tả thống nhất với chính sách đã duyệt.
- Việc lưu bằng chứng khách hàng chấp thuận **phiên bản điều khoản cụ thể vào từng hợp đồng**
  chưa nằm trong patch này; cần thiết kế EventStore schema và migration riêng trước
  khi khẳng định đáp ứng yêu cầu chứng từ pháp lý.

Migration `down` chỉ hạ bản seed xuống `INACTIVE` và bỏ unique index,
**không xóa** collection hoặc hợp đồng/số dư. Chỉ rollback có kiểm soát.
