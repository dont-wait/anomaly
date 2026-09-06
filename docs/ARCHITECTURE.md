# Server Architecture

## Overview

`server/` hiện có 2 executable chính:

- `cmd/api`: HTTP API cho account và media
- `cmd/worker`: projection worker đọc event từ KurrentDB và cập nhật read model ở MongoDB

`cmd/relay` hiện chưa có implementation hoàn chỉnh.

## Layers

- `internal/domain`: cấu hình dùng chung và domain model
- `internal/application`: use case cho account, tách command và query
- `internal/infrastructure`: adapter cho MongoDB, KurrentDB, RustFS
- `internal/presentation/http`: router, handler, middleware, helper trả response HTTP
- `internal/composition`: wiring dependency để tạo handler

## Runtime Flow

### API

1. `cmd/api/main.go` load `.env` và build config
2. Kết nối MongoDB, KurrentDB, RustFS
3. `rustfs.EnsureBucket(...)` đảm bảo bucket media tồn tại trước khi serve
4. Tạo repository và handler qua `internal/composition`
5. Đăng ký route qua `internal/presentation/http/router.go`
6. Serve HTTP ở cổng `8080`

### Worker

1. `cmd/worker/main.go` load config và kết nối MongoDB + KurrentDB
2. Subscribe vào `$all`
3. Chỉ xử lý event account đã biết
4. Replay aggregate từ event store
5. Upsert read model sang MongoDB
6. Lưu checkpoint để resume khi restart hoặc reconnect

## Storage Responsibilities

- MongoDB: read model account và checkpoint projection
- KurrentDB: source of truth cho event account
- RustFS: binary/media object storage
- Redis, Kafka: đã có trong local infra nhưng chưa thấy business flow chính dùng trực tiếp trong code hiện tại

## HTTP Surface

Account routes:

- `POST /api/accounts`
- `GET /api/accounts`
- `GET /api/accounts/{id}`
- `GET /api/accounts/by-email/{email}`

Media routes:

- `POST /api/media/upload`
- `GET /api/media/download?key=...`

Utility route:

- `GET /health`
