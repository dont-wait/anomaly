# Server Features

## Accounts

Hiện backend hỗ trợ các luồng account cơ bản:

- tạo account mới
- lấy toàn bộ account
- lấy account theo `id`
- lấy account theo `email`

Write model đi qua event store, còn read model được materialize sang MongoDB bởi worker.

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
