# Server Infrastructure

## Local Stack

`docker-compose.yml` hiện dựng các service sau:

- `anomaly-server`: HTTP API
- `anomaly-worker`: projection worker
- `rustfs`: object storage cho media
- `mongo`: read model và checkpoint store
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

RUSTFS_ENDPOINT=http://localhost:9000
RUSTFS_ACCESS_KEY=your_rustfs_access_key
RUSTFS_SECRET_KEY=your_rustfs_secret_key
RUSTFS_BUCKET=media
RUSTFS_REGION=us-east-1
```

Ngoài ra code còn hỗ trợ:

```env
EVENT_STORE_CONN_STRING=kurrentdb://localhost:2113?tls=false
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
go run ./cmd/worker
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
