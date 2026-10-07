# Server Infrastructure

## Local Stack

`docker-compose.yml` hiện dựng các service sau:

- `anomaly-server`: HTTP API
- `anomaly-worker`: banking projector (`/app/projector`)
- `rustfs`: object storage cho media
- `mongo`: read projections và checkpoint
- `eventstore`: event store
- `redis`: cache hoặc infra phụ trợ
- `kafka`: streaming broker
- `kafka-ui`: UI quan sát Kafka

## Default Ports

- API: `8080`
- Health: `8080/health`
- EventStore HTTP/UI: `2113`
- EventStore gossip/admin: `8081`
- MongoDB: `27017`
- Redis: `6379`
- Kafka: `9092`
- Kafka UI: `8082`
- RustFS API: `9000`
- RustFS console: `9001`

## Environment Variables

Từ `server/.env.example`:

```env
MONGO_URI=mongodb://localhost:27017
MONGO_DB=anomaly
MONGO_ROOT_USERNAME=username
MONGO_ROOT_PASSWORD=password
CORS_ALLOWED_ORIGINS="http://localhost:1420,http://localhost:5173,http://localhost:3000,tauri://localhost,http://tauri.localhost"
SWAGGER_ENABLED=false

RUSTFS_ENDPOINT=http://localhost:9000
RUSTFS_ACCESS_KEY=your_rustfs_access_key
RUSTFS_SECRET_KEY=your_rustfs_secret_key
RUSTFS_BUCKET=media
RUSTFS_REGION=us-east-1
```

Ngoài ra code còn hỗ trợ:

```env
EVENT_STORE_CONN_STRING=esdb://localhost:2113?tls=false
```

## Common Workflows

Chạy full local stack:

```bash
docker compose up --build
```

Dừng stack:

```bash
docker compose down
```

Xóa luôn volumes:

```bash
docker compose down -v
```

Chạy API local không cần full stack:

```bash
go mod download
go run ./cmd/api
```

Chạy worker:

```bash
go run ./cmd/projector
```

## Verification

Chạy toàn bộ test backend:

```bash
go test ./...
```

Chạy riêng RustFS tests:

```bash
go test ./internal/infrastructure/rustfs -v
```

## Banking Event Operations

EventStoreDB là source of truth; API, seed và banking projector kết nối
EventStoreDB. Migration `000005` bổ sung index cho projections.

Với legacy account event streams, `make import-bank` lấy canonical account từ
EventStoreDB. Với MVP từng ghi thẳng Mongo, đối soát/sao lưu dữ liệu rồi chọn rõ
`make import-mongo-bank`. Cả hai chỉ import offline vào banking stream trống,
không ghi đè canonical history hiện có. Dừng API/writers/projectors trước cutover.

Chạy một banking projector cho checkpoint của nó; dừng projector trước khi reset
checkpoint để replay. Pipeline training thuộc repo riêng. EventStoreDB có volume
`eventstore-data`; sao lưu/copy dữ liệu của container cũ không có volume trước
khi recreate.

[Lệnh migration, cutover và replay](../server/README.md#eventstore-first-banking).
