# Server Architecture

## Overview

`server/` có các executable nghiệp vụ sau:

- `cmd/api`: HTTP API cho account, media và transfer
- `cmd/projector`: dựng account, KYC, balance, ledger và feed từ stream banking
- `cmd/worker`: legacy account projector, không thuộc write path hiện tại
- `cmd/import-bank`: cutover offline vào banking stream trống
- `cmd/relay`: persistent subscription từ EventStoreDB và Kafka producer

`consumer-mock/` là Kafka consumer giả lập độc lập cho model pipeline, chạy ngoài `server/`.
`server/ml-pipeline/` là publisher Python chạy riêng: consume trigger
`TransferCreated` từ Kafka, đọc event gốc + history từ EventStoreDB, chạy bundle
GATv2 local, publish risk response và training record.

## Layers

- `internal/domain`: cấu hình dùng chung và domain model
- `internal/application`: use case cho account, tách command và query
- `internal/infrastructure`: adapter cho MongoDB, KurrentDB, Kafka và RustFS
- `internal/presentation/http`: router, handler, middleware, helper trả response HTTP
- `internal/composition`: wiring dependency để tạo handler

## Runtime Flow

### API

1. `cmd/api/main.go` load `.env` và build config
2. Kết nối EventStoreDB, MongoDB, Redis và RustFS
3. Kiểm tra canonical banking stream và đảm bảo bucket media tồn tại
4. Tạo repository và handler qua `internal/composition`
5. Đăng ký route qua `internal/presentation/http/router.go`
6. Serve HTTP ở cổng `8080`

### Worker

1. `cmd/projector` kết nối MongoDB và EventStoreDB
2. Subscribe vào stream `banking` từ checkpoint đã commit
3. Dựng account/KYC, balance, transaction, ledger và feed
4. Ghi projection cùng checkpoint trong một Mongo transaction
5. Retry/replay không ghi nhận debit hai lần

### Kafka relay

1. `cmd/relay` tạo persistent subscription `kafka-publisher` trên stream `banking`
2. Đọc event từ EventStoreDB và tạo envelope versioned
3. Với `TransferCompleted`, đọc feature state theo source account và enrich
   envelope bằng các feature model
4. Publish envelope vào Kafka topic `anomaly.bank.events.v1`
5. Cập nhật feature state sau khi Kafka xác nhận thành công rồi ACK EventStoreDB
6. NACK retry khi Kafka hoặc feature state lỗi; payload loại bỏ các field nhạy
   cảm như password

Feature state được lưu trong MongoDB bằng materialized view tăng dần, không query
toàn bộ transaction history cho mỗi event. `consumer-mock` đọc `model_features`
cùng account number, authentication và metadata thiết bị từ envelope. Client gửi
`X-Device-ID` và `X-Platform`; giá trị thiếu được chuyển thành `UNKNOWN`.

Publisher GATv2 tại `server/ml-pipeline` không dùng payload Kafka để dựng feature
vì relay đã pseudonymize account/device ID. Service này dùng `TransferCreated` trên
Kafka làm trigger, đọc raw event + toàn bộ transfer history trước `occurredAt` từ
EventStoreDB, rồi chạy `ReplayPredictor` trên bundle GATv2 đã export. Kết quả được
publish vào `anomaly.bank.risk.v1`; training record được ghi JSONL và publish vào
`anomaly.model.training.v1`. Record này chứa request gốc + response + timestamp,
không phải label.

Chi tiết contract và state lifecycle: [Model feature enrichment](MODEL_FEATURES.md).

## Storage Responsibilities

- EventStoreDB: source of truth cho account, KYC, số dư và transaction
- MongoDB: projection có thể rebuild cho account, KYC, ledger, feed và checkpoint
- RustFS: binary/media object storage
- Redis: OTP cho auth và transfer
- Kafka: event bus cho model pipeline và các consumer downstream

## HTTP Surface

JSON success responses use `{ "status": number, "message": string, "data": object | array }`.
JSON errors use `{ "status": number, "title": string, "errors": array }`.
Binary media downloads are returned without a JSON envelope.

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

## Banking Events

Command dựng canonical aggregate từ stream `banking`, kiểm tra nghiệp vụ và
append với expected revision. Một `TransferCompleted` ghi nhận cả debit/credit;
revision chung chặn hai transfer cùng chi tiêu vượt số dư. Mongo không quyết định
funds hoặc trạng thái command. API confirm trả từ canonical event state, còn paging
history và summary lấy projection eventual consistency.

Projector ghi projection + checkpoint atomic. Repo training riêng đọc envelope
transaction từ EventStoreDB và tự quản lý pipeline/checkpoint của nó. Register
và KYC cũng đi qua canonical stream để rebuild được toàn bộ business read model.
Các adapter/worker account stream cũ được giữ cho legacy tooling.

Contract, giới hạn stream chung, migration/cutover và workflow:
[EventStore-first banking](../server/README.md#eventstore-first-banking).
