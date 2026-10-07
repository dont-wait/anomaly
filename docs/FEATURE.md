# Server Features

## Accounts

Hiện backend hỗ trợ các luồng account cơ bản:

- tạo account mới
- lấy toàn bộ account
- lấy account theo `id`
- lấy account theo `email`

Register và KYC ghi event vào EventStoreDB. Banking projector dựng account/KYC trên MongoDB; số dư canonical được quản lý bởi các event tài chính.

## Media Storage

Media được lưu trên RustFS qua `internal/infrastructure/rustfs`.

Hiện có 2 API:

- upload media theo `key`
- download media theo `key`

Hành vi hiện tại:

- giới hạn upload multipart tối đa `32 MiB`
- thiếu `key` sẽ trả `400`
- object không tồn tại sẽ map thành `404`
- nếu object không có `Content-Type`, response fallback về `application/octet-stream`

Lưu ý hiện chưa có:

- authentication/authorization cho media route
- validation định dạng file theo business rule
- antivirus scan hoặc content inspection

## Testing Status

### RustFS Repository

Đã có test cho `internal/infrastructure/rustfs`:

- upload/download/delete trả lỗi khi RustFS không reachable
- `Download` map `NoSuchKey` thành `ErrObjectNotFound`
- `Download` fallback `Content-Type`
- integration test end-to-end upload -> download -> delete với file tạm thật

Chạy riêng:

```bash
go test ./internal/infrastructure/rustfs -v
```

E2E hiện tại là ở mức repository integration với RustFS. Chưa có end-to-end xuyên suốt từ HTTP handler -> service -> repository -> RustFS.

## Transactions

- Transfer nội bộ có OTP/idempotency, kiểm tra số dư từ EventStoreDB.
- Một event hoàn tất ghi nhận cả debit/credit, với expected revision chung.
- Lifecycle gồm created, OTP timing updated, completed và cancelled.
- Mongo balance, ledger, history/feed và checkpoint là projection atomic.
- API confirm có thể trả thành công khi projection chưa cập nhật.
- Repo training riêng có thể đọc canonical transaction event envelope từ EventStoreDB.
- E2E kiểm chứng competing transfers khi Mongo trống và rebuild toàn bộ read model.

Xem [contract và workflow](../server/README.md#eventstore-first-banking).
