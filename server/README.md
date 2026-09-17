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

### Seed tài khoản demo

Sau khi MongoDB và migration đã chạy:

```bash
export APP_ENV=development
export SEED_DEMO_ENABLED=true
make seed
```

Trên PowerShell:

```powershell
$env:APP_ENV = "development"
$env:SEED_DEMO_ENABLED = "true"
make seed
```

`make seed` gọi runner chung, chạy tất cả seeder đã đăng ký trong
`internal/seeder/`. Go tự biên dịch các file `.go` thuộc package này
(không chạy `_test.go`); Makefile không cần liệt kê từng file.

Để thêm seeder, tạo file trong `internal/seeder/`, khai báo `package seeder`
và tự đăng ký trong `init()`:

```go
func init() {
    seeders.register("020_example", seedExample)
}

func seedExample(ctx context.Context, deps Dependencies) error {
    // Tạo dữ liệu theo cách chạy lại không sinh bản ghi trùng.
    return nil
}
```

Thêm import `context` cho file mới. Runner chạy theo tên tăng dần
(`010_accounts`, `020_example`, ...) để kiểm soát thứ tự phụ thuộc, dừng ngay
và trả tên seeder khi gặp lỗi. Tên trùng bị từ chối. File helper không đăng ký
thì không được chạy riêng. Không cần sửa Makefile, `cmd/seed` hoặc danh sách
trong runner khi thêm seeder vào cùng package. Nếu cần repository mới, bổ sung
vào `Dependencies` và khởi tạo tại `cmd/seed`.

Dữ liệu và logic tài khoản nằm trong `internal/seeder/accounts.go`; thêm phần tử
vào `demoAccounts()` với CCCD, email, username và idempotency key riêng để thêm
tài khoản. `cmd/seed` chỉ khởi tạo kết nối, dependency và gọi runner.

Tài khoản demo: CCCD `079123456789`, username `demo.customer`, email
`demo.customer@example.com`, mật khẩu `DemoLocal@123`, số dư `128.540.000 VND`.
Đây là dữ liệu công khai chỉ dùng cho local/dev. Lệnh yêu cầu `APP_ENV` là
`development`, `dev`, hoặc `local` và `SEED_DEMO_ENABLED=true`.

Seed đi qua command đăng ký của ứng dụng. Chạy lại không tạo tài khoản trùng;
tài khoản khớp sẽ được đưa về số dư demo. Nếu thông tin hoặc mật khẩu tài khoản
đã tồn tại khác dữ liệu seed, lệnh báo lỗi thay vì ghi đè. Tài khoản từng seed
bằng `SEED_*` trước đây cũng phải khớp dữ liệu trong code để chạy lại thành công.
Dashboard lấy profile bằng `GET /api/auth/me` sau khi login.

### 4. Chạy worker local

```bash
go run ./cmd/worker
```

### MongoDB migrations

Runner đọc `migrations/` từ thư mục làm việc hiện tại; chạy các lệnh dưới đây
tại `server/`. Mỗi lần chạy đều in đường dẫn tuyệt đối và tên database. Thư mục
rỗng hoặc thiếu migration hợp lệ sẽ báo lỗi. `migrate-status` suy ra `applied`
và `pending` từ version trong database, không so sánh schema hoặc lưu lịch sử
từng file. Khi dirty, migration tại version hiện tại được đánh dấu `dirty`,
các migration khác là `unknown`. Compose có service `anomaly-migrate` chạy
`up` tự động trước các service phụ thuộc.

Ba cặp JSON migration tạo collection, JSON Schema validator và index cho
`customers`, `kyc_sessions`, và `accounts`. Validator bám theo BSON record mà
backend đang ghi, gồm nested object, nullable field, enum và kiểu tham chiếu
`ObjectId`:

```bash
make migrate          # apply all pending migrations
make migrate-up-one   # apply the next pending migration
make migrate-version  # show current version and dirty state
make migrate-status   # list migration states and resolved directory
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
