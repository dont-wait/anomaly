# Anomaly Server

Backend cho Anomaly, gồm HTTP API, banking projection worker và adapter hạ tầng local.

## Docs Map

- `README.md`: quick start và chỉ mục tài liệu
- [Architecture](../docs/ARCHITECTURE.md): cấu trúc layer, executable, và runtime flow
- [Transaction API for frontend](docs/transactions-api.md): request/response, OTP, retry, filters và mã lỗi
- [Features](../docs/FEATURE.md): feature hiện có và phạm vi test
- [Infrastructure](../docs/INFRA.md): service local, env, port, workflow vận hành

## Overview

Stack hiện tại:

- API server: Go
- Canonical account/financial store: EventStoreDB
- Read projections: MongoDB
- Event store: EventStoreDB 23 for legacy projections and transaction lifecycle events
- Media storage: RustFS
- Local infra: Docker Compose

EventStoreDB is the source of truth for registration, KYC, balances and transfers.
Commands rebuild the banking aggregate from the `banking` stream and append with
its expected revision. MongoDB stores rebuildable read projections:

- `customers`, `kyc_sessions`, `accounts`: account/KYC and financial projections
- `transactions`, `ledger_entries`, `account_transaction_feed`: transaction history
- `checkpoints`: banking projector position

Balances use whole `int64` VND values (BSON `long` in MongoDB). Migration `000005`
creates transaction collections, including the legacy `outbox_events`; the active
write path never writes that outbox. Migration `000006` adds banking projection
indexes and the financial account reference. Mongo projections commit their
updates and checkpoint in one transaction, requiring a replica set or sharded
MongoDB. Compose initializes local `rs0`.

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
- Swagger UI: `http://localhost:8080/swagger/` when `SWAGGER_ENABLED=true`
- OpenAPI spec: `http://localhost:8080/openapi.json` when `SWAGGER_ENABLED=true`
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

Transaction confirmation cần MongoDB multi-document transaction nên Mongo phải
là replica set hoặc sharded cluster. Local Compose đã cấu hình single-node
replica set `rs0`; khi chạy API từ host, `MONGO_URI` phải trỏ tới replica set
member mà host phân giải và truy cập được.

```bash
go mod download
make migrate
go run ./cmd/api
```

Để bật Swagger khi chạy local, thêm vào `server/.env`:

```env
SWAGGER_ENABLED=true
```

Swagger bị tắt mặc định. Khi chạy Docker Compose, truyền biến tương tự trước
khi start stack:

```bash
SWAGGER_ENABLED=true docker compose --profile app up --build
```

### Thêm API mới

Các route HTTP phải được đăng ký qua `openapi.Registry` trong file `*_routes.go`.
Registry vừa đăng ký route vào `http.ServeMux`, vừa sinh OpenAPI từ metadata và
request/response struct. Vì vậy không chỉnh sửa `openapi.json` thủ công; khi
thêm API chỉ cần khai báo operation, schema, auth và status code ngay tại route.

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
Tài khoản quản trị local: CCCD `001234567890`, username `admin.staff`, mật khẩu
`admin123`.
Đây là dữ liệu công khai chỉ dùng cho local/dev. Lệnh yêu cầu `APP_ENV` là
`development`, `dev`, hoặc `local` và `SEED_DEMO_ENABLED=true`.

Seed đi qua command đăng ký của ứng dụng. Chạy lại không tạo tài khoản trùng;
tài khoản khớp sẽ được đưa về số dư demo. Nếu thông tin hoặc mật khẩu tài khoản
đã tồn tại khác dữ liệu seed, lệnh báo lỗi thay vì ghi đè. Seed ghi account và
số dư qua EventStoreDB; chạy projector để cập nhật MongoDB. Tài khoản từng seed
bằng `SEED_*` trước đây cũng phải khớp dữ liệu trong code để chạy lại thành công.
Thông báo lỗi liệt kê field không khớp; cần sửa hoặc xóa dữ liệu local tương ứng
trước khi chạy lại, seeder không tự nâng một account `user` thành `admin`.
Dashboard lấy profile bằng `GET /api/auth/me` sau khi login.

### 4. Chạy worker local

```bash
go run ./cmd/projector
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

JSON success responses use `{ "status": number, "message": string, "data": object | array }`.
JSON errors use `{ "status": number, "title": string, "errors": array }`.
Binary media downloads are returned as binary responses instead of JSON.

Example error response:

```json
{
  "status": 400,
  "title": "Bad Request",
  "errors": [
    {
      "code": "INVALID_EMAIL",
      "field": "email",
      "detail": "invalid email"
    }
  ]
}
```

Account:

- `POST /api/auth/register`
- `POST /api/auth/login`
- `GET /api/auth/me`
- `POST /api/accounts/{id}/verify`
- `GET /api/accounts`
- `GET /api/accounts/{id}`
- `GET /api/accounts/by-email/{email}`

`GET /api/accounts` chỉ dành cho admin. Hai endpoint tra cứu account cho phép
admin hoặc chính chủ. Role được ký trong JWT; thay đổi role trong database có
hiệu lực với token phát hành mới, token cũ còn hiệu lực đến thời điểm `exp`.

Media:

- `POST /api/media/upload`
- `GET /api/media/download?key=...`

Transactions (authenticated; transfer MVP supports internal accounts only):

- `GET /api/accounts/lookup?bankCode=ANOMALY&accountNo=...`
- `GET /api/transfers/recent-recipients`
- `POST /api/transfers`
- `POST /api/transfers/{transferId}/confirm`
- `POST /api/transfers/{transferId}/otp/resend`
- `GET /api/transactions`
- `GET /api/transactions/summary?month=YYYY-MM`
- `GET /api/transactions/{id}`

Health:

- `GET /health`

## Verification

Chạy toàn bộ test backend:

```bash
make test
```

Các test Go mặc định không yêu cầu MongoDB hoặc EventStoreDB thật.

Chạy account end-to-end test; Testcontainers tự khởi động và dọn dẹp MongoDB
cô lập:

```bash
make test-account-e2e
```

Chạy API end-to-end tests bằng Hurl sau khi API, MongoDB và RustFS đã sẵn sàng:

```bash
make test-api
```

Suite yêu cầu admin seed đã tồn tại. Mặc định dùng CCCD `001234567890` và mật
khẩu `admin123`; có thể override bằng `ADMIN_CCCD` và `ADMIN_PASSWORD`.

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

## EventStore-first banking

```text
Register / KYC / transfer command
  -> rebuild canonical aggregate from EventStoreDB banking stream
  -> validate ownership, idempotency, status and current funds
  -> append event with expected revision
       -> banking projector -> Mongo read models + checkpoint (atomic)
       -> external training repository consumes transfer event envelopes
```

A single banking stream is the MVP's concurrency boundary. Both sides of an
internal transfer commit in one `TransferCompleted` event. If another command
appends first, the command rebuilds and validates again before retrying. Concurrent
transfers cannot spend the same remaining balance twice even when Mongo is stale
or empty. The API never falls back to Mongo for command state or funds. Register,
login/account queries and KYC use the same canonical repository. Paginated
transaction history, recent recipients and summaries read the Mongo projection
and are eventually consistent; confirm/detail responses use canonical transfer
state, so a successful append does not depend on projection catching up.

Registration uniqueness is also checked under that expected-revision boundary.
New account numbers are deterministic 14-digit numbers, matching the transfer
lookup contract. Public 24/32-character account IDs remain unchanged; a stable
`financial_id` maps them to ObjectID ledger/feed references. Profile version and
financial projection version are separate, and profile/KYC updates preserve
current funds. Development seed funding emits `BankBalanceSeeded` after the
existing `APP_ENV`/`SEED_DEMO_ENABLED` checks; there is no public balance setter.

Canonical events include `BankAccountCreated`, `BankAccountUpdated`,
`BankBalanceSeeded`, and the transfer lifecycle:

- `TransferCreated`: sequence 1, awaiting OTP
- `TransferOTPUpdated`: subsequent OTP timing updates, without the OTP code
- `TransferCompleted`: one atomic debit/credit with before/after balance snapshots
- `TransferCancelled`: terminal cancellation

Transfer sequences advance for each lifecycle event. Stable IDs make terminal
command retries idempotent. An append timeout can have an uncertain outcome;
retry the same command/idempotency key so replay resolves whether it committed.
The canonical record carries the immutable transaction snapshot required to
rebuild the API's fields. The separate training repository can consume the sanitized
transaction event envelope:
account financial IDs, amount, fee, currency, channel, status, sequence, event
ID/time and balance snapshots, without profile credentials, names, notes or OTPs.
Channel remains `web`, matching the current API's existing behavior.


### Migration numbering after merge

Thứ tự hiện tại: `000004_add_account_role`, `000005_create_transaction_collections`,
`000006_transaction_event_pipeline`. Nếu đã chạy bản nhánh cũ có transaction ở
version 4 hoặc pipeline ở version 5, cần đối soát schema/version trước cutover;
runner chỉ lưu version, không nhận biết migration cùng số đã đổi nội dung.
Không tự chạy `down` hoặc `force` trên database đó.

### New database

Run in `server/` with EventStoreDB and MongoDB `rs0` available:

```bash
make migrate-up
make start-worker       # banking projector
# Start API using the existing auth, Redis, SMTP and media configuration:
make start-api
```

Compose's `anomaly-worker` now runs `/app/projector`. API and seed also connect
to EventStoreDB. The old
`cmd/worker`/`account-*` stream adapter is retained for legacy tooling and is not
used by the active banking write path. Do not run that legacy projector alongside
the new banking projector during cutover.

### Existing direct-Mongo MVP data

The previous MVP wrote accounts/transfers directly to Mongo. It requires an
explicit offline cutover before using this new canonical stream:

1. Stop API, seeders, legacy workers and all projectors; back up the
   database and existing EventStoreDB data, and reconcile the old balances.
2. Apply migrations through `000006`, then run `make import-mongo-bank` while writers remain
   stopped. This explicitly invokes `cmd/import-bank -offline -source mongo`.
3. Start the new banking projector and then the API.

Import writes one `BankImported` snapshot and historical transfer records into an
**empty** `banking` stream in one append batch. Current account balances are the
explicit cutover baseline; old completion events retain their original ledger
snapshots and are marked historical so those debits are not applied twice.
Import is the only bridge from old Mongo data: normal commands never read Mongo
as authoritative state. An identical import can be retried; an existing unrelated
canonical stream is never overwritten. Incomplete/unsupported historical data
fails validation. Imports above 3 MiB are rejected before append and need a
separate staged migration; this command intentionally handles the small MVP
cutover only. Legacy `outbox_events` remain archival and are not published.

If an older deployment already has authoritative `account-*` events and no
unlogged Mongo transfers, use `make import-bank` instead. Its default source is
EventStoreDB: it migrates those canonical account aggregates into the banking
stream. It refuses unlogged Mongo transfers rather than silently overriding the
canonical balances. Reconcile mixed histories before explicitly choosing
`-source mongo`; no automatic merge or source fallback occurs. The API refuses
startup when the new canonical stream is missing but Mongo accounts or legacy
authoritative account streams exist.

EventStoreDB now persists `/var/lib/eventstore` in `eventstore-data`. When
upgrading a container that had no volume, copy/backup its existing data before
recreating it with this Compose configuration. No live data migration is performed
by a Go build or unit test.

### Banking replay

Run one banking projector for its checkpoint. Stop the running projector before
resetting its checkpoint:

```bash
go run ./cmd/projector -replay
```

Banking replay reconstructs customers, KYC, account balances, transactions, ledger
and feed from the canonical journal. Delivery/restart retries do not debit again;
projection and checkpoint commit together. Errors leave the committed checkpoint
unchanged. Resetting a checkpoint replays into existing rows; to rebuild from an
empty read model, clear the business projections offline first and retain their
schema/indexes. Do not delete EventStoreDB data to rebuild MongoDB.

### External training repository

Model training lives in a separate repository. This repo owns the canonical
journal and business projections; it does not run a training consumer, store a
training dataset or export features.

The external consumer subscribes to `banking`, filters the four transfer lifecycle
record types listed above, and reads the record's `event` envelope. Its contract
contains `eventId`, `transactionId`, `eventType`, `schemaVersion`, `sequence`,
`occurredAt` and `payload`. Consume that envelope rather than the private `account`
or `transfer` snapshots used to rebuild API state. The training repo owns its
checkpoint, dataset, feature extraction, labels and model pipeline.

Offline cutover records carry `historical: true`; their balance snapshots describe
past transfers, while the imported account baseline already includes those transfers.

### Validation and limits

```bash
make test
make test-transaction-e2e
```

E2E starts isolated MongoDB `rs0` and EventStoreDB instances. It checks canonical
registration/KYC/funding without Mongo writes, competing transfers while Mongo is
empty, command idempotency and ownership, subscription resume, projection rollback
with unchanged checkpoint, rebuilding the full read model, offline import
without double debit and EventStore failures
without a Mongo fallback. It does not exercise SMTP delivery or the installed
mobile application.

The MVP rebuilds the single stream for command reads and serializes its writes.
This prioritizes correct funds/uniqueness invariants; throughput and replay cost
will grow with the journal. Production scaling needs EventStore snapshots and a
carefully designed account reservation/settlement boundary before splitting the
stream. MongoDB must remain a projection after that change.
