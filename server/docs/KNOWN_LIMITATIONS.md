# Known Limitations (Hạn chế đã biết)

Ghi lại các hạn chế kỹ thuật đã biết nhưng chưa xử lý — dùng để giải trình
khi bảo vệ khóa luận, và làm checklist việc cần làm nếu phát triển tiếp.

## 1. Chuyển tiền chưa atomic hoàn toàn

**Vị trí:** `internal/application/transaction/commands/transfer.go`

**Vấn đề:** Thao tác trừ tiền (source), cộng tiền (dest), ghi bản ghi
`transaction` + 2 `feed entry` được thực hiện qua **nhiều lệnh ghi Mongo
riêng biệt**, không nằm trong 1 transaction chung. Nếu app bị crash hoặc
mất kết nối Mongo giữa chừng, có thể để lại trạng thái dở dang (vd: đã trừ
tiền người gửi nhưng chưa kịp cộng cho người nhận).

**Đã giảm thiểu:** Có cơ chế optimistic concurrency (kiểm tra `version`
trước khi ghi đè) để tránh 2 giao dịch đồng thời ghi đè mất số dư của nhau
— nhưng không giải quyết vấn đề "ghi dở dang khi crash" nêu trên.

**Cách fix đúng:** 
1. Đổi Mongo sang chạy dạng **replica set** (kể cả 1 node) trong
   `docker-compose.yml` — multi-document transaction của Mongo bắt buộc
   cần điều kiện này.
2. Bọc 4 lệnh ghi (source, dest, transaction, feed) trong 1
   `session.WithTransaction(...)`.

**Mức độ ưu tiên:** Trung bình — với quy mô khóa luận (demo, không phải
production thật với lượng giao dịch lớn), rủi ro thực tế thấp vì Mongo
hiếm khi crash giữa 2 lệnh ghi liên tiếp trong môi trường dev/demo.