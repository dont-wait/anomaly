# Anomaly Server

Backend cho Anomaly, gồm HTTP API, projection worker, và adapter hạ tầng local.

## Docs Map

- `README.md`: quick start và chỉ mục tài liệu
- `ARCHITECTURE.md`: cấu trúc layer, executable, và runtime flow
- `FEATURE.md`: feature hiện có và phạm vi test
- `INFRA.md`: service local, env, port, workflow vận hành

## Overview

Stack hiện tại:

- API server: Go
- Read model: MongoDB
- Event store: KurrentDB
- Media storage: RustFS
- Local infra: Docker Compose

## Quick Start

### 1. Tạo file môi trường

Từ `server/`:

```bash
cp .env.example .env
```

Cần điền ít nhất:

```env
MONGO_ROOT_USERNAME=username
MONGO_ROOT_PASSWORD=password
RUSTFS_ACCESS_KEY=your_rustfs_access_key
RUSTFS_SECRET_KEY=your_rustfs_secret_key
```

### 2. Chạy full local stack

```bash
docker compose --profile app up --build
```

Endpoint chính:

- API: `http://localhost:8080`
- Health: `http://localhost:8080/health`
- RustFS API: `http://localhost:9000`
- RustFS console: `http://localhost:9001`
- MongoDB: `localhost:27017`
- Kafka UI: `http://localhost:8082`
- KurrentDB UI/API: `http://localhost:2113`
- RustFS API: `http://localhost:9000`
- RustFS Console: `http://localhost:9001`
- RustFS Admin Client: `docker compose exec rustfs-admin aws s3 ls --endpoint-url http://rustfs:9000`

### 3. Chạy API local

Nếu chỉ chạy API bằng môi trường local sẵn có:

```bash
go mod download
go run ./cmd/api
```

### 4. Chạy worker local

```bash
go run ./cmd/worker
```

## API Summary

Account:

- `POST /api/auth/register`
- `POST /api/auth/login`
- `GET /api/auth/me`
- `POST /api/accounts/{id}/verify`
- `GET /api/accounts`
- `GET /api/accounts/{id}`
- `GET /api/accounts/by-email/{email}`

Media:

- `POST /api/media/upload`
- `GET /api/media/download?key=...`

Health:

- `GET /health`

## Verification

Chạy toàn bộ test backend:

```bash
make test
```

Các test Go không khởi động hoặc yêu cầu database thật.

Chạy API end-to-end tests bằng Hurl sau khi API, projection worker, MongoDB,
KurrentDB và RustFS đã sẵn sàng:

```bash
make test-api
```

Đổi endpoint khi cần:

```bash
make test-api BASE_URL=http://localhost:18080
```

Suite Hurl tạo account và media object riêng với ID ngẫu nhiên, kiểm tra toàn bộ
luồng register -> projection -> login -> JWT -> verify và upload -> download.
Nếu một dependency thật không hoạt động, lệnh sẽ trả về exit code khác `0`.

Chạy riêng test RustFS:

```bash
go test ./internal/infrastructure/rustfs -v
```

RustFS hiện đã có integration test end-to-end ở mức repository cho luồng upload -> download -> delete với file tạm thật.
