# Anomaly Server

Backend cho Anomaly, gồm HTTP API, projection worker, và adapter hạ tầng local.

## Docs Map

- `README.md`: quick start và chỉ mục tài liệu
- [Architecture](../docs/ARCHITECTURE.md): cấu trúc layer, executable, và runtime flow
- [Features](../docs/FEATURE.md): feature hiện có và phạm vi test
- [Infrastructure](../docs/INFRA.md): service local, env, port, workflow vận hành

## Overview

Stack hiện tại:

- API server: Go
- Account store: MongoDB
- Event store: EventStoreDB 23 for legacy projections and future transaction events
- Media storage: RustFS
- Local infra: Docker Compose

MongoDB stores account data directly in three business collections:

- `customers`: customer profile, identity, credit profile, and current KYC status
- `kyc_sessions`: append-style history for each KYC attempt and its media metadata
- `accounts`: financial account, balance, status, and authentication credentials

Account balances are stored as BSON `Decimal128`; domain operations currently use
whole `int64` VND values and convert at the MongoDB boundary. Registration and
verification do not append account lifecycle events. The legacy projection worker
uses two technical collections:

- `checkpoints`: resume EventStoreDB subscriptions after restart
- `projection_failures`: dead-letter permanent projection errors before advancing the checkpoint

There is no `ledger_entries` collection yet.

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

Compose chạy `golang-migrate` trước API và worker. Trạng thái migration được lưu
trong MongoDB collection `schema_migrations`.

Endpoint chính:

- API: `http://localhost:8080`
- Health: `http://localhost:8080/health`
- RustFS API: `http://localhost:9000`
- RustFS console: `http://localhost:9001`
- MongoDB: `localhost:27017`
- Kafka UI: `http://localhost:8082`
- EventStoreDB UI/API: `http://localhost:2113`
- RustFS API: `http://localhost:9000`
- RustFS Console: `http://localhost:9001`
- RustFS Admin Client: `docker compose exec rustfs-admin aws s3 ls --endpoint-url http://rustfs:9000`

### 3. Chạy API local

Nếu chỉ chạy API bằng môi trường local sẵn có:

```bash
go mod download
make migrate
go run ./cmd/api
```

### 4. Chạy worker local

```bash
go run ./cmd/worker
```

### MongoDB migrations

Ba cặp JSON migration tạo collection, JSON Schema validator và index cho
`customers`, `kyc_sessions`, và `accounts`. Validator bám theo BSON record mà
backend đang ghi, gồm nested object, nullable field, enum và kiểu tham chiếu
`ObjectId`:

```bash
make migrate          # apply all pending migrations
make migrate-up-one   # apply the next pending migration
make migrate-version  # show current version and dirty state
make migrate-down     # roll back the latest migration
```

Trên Windows có thể chạy native bằng PowerShell hoặc Command Prompt, không cần
cài `make`:

```powershell
.\scripts\windows\migrate.ps1 up
.\scripts\windows\migrate.ps1 up-by-one
.\scripts\windows\migrate.ps1 version
.\scripts\windows\migrate.ps1 down
```

```bat
scripts\windows\migrate.cmd up
scripts\windows\migrate.cmd up-by-one
scripts\windows\migrate.cmd version
scripts\windows\migrate.cmd down
```

Nếu đã cài GNU Make trên Windows, các target `make migrate`,
`make migrate-up-one`, `make migrate-version`, và `make migrate-down` vẫn dùng
y như Linux/macOS. Makefile tự nhận `OS=Windows_NT` và gọi PowerShell script.

Hai script dùng cùng `MONGO_URI` và `MONGO_DB` mà backend đọc từ environment
hoặc file `.env`; không có connection string hard-code riêng cho Windows.

`migrate-down` tắt validator và xóa các index do migration mới nhất tạo; dữ liệu
trong collection được giữ nguyên.

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

Chạy account end-to-end test; Testcontainers tự khởi động và dọn dẹp MongoDB
cô lập:

```bash
make test-account-e2e
```

Chạy API end-to-end tests bằng Hurl sau khi API, MongoDB và RustFS đã sẵn sàng:

```bash
make test-api
```

Đổi endpoint khi cần:

```bash
make test-api BASE_URL=http://localhost:18080
```

Suite Hurl tạo account và media object riêng với ID ngẫu nhiên, kiểm tra toàn bộ
luồng register -> MongoDB -> login -> JWT -> verify và upload -> download.
Nếu một dependency thật không hoạt động, lệnh sẽ trả về exit code khác `0`.

Chạy riêng test RustFS:

```bash
go test ./internal/infrastructure/rustfs -v
```

RustFS hiện đã có integration test end-to-end ở mức repository cho luồng upload -> download -> delete với file tạm thật.
