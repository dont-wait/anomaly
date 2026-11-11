# Anomaly

Monorepo cho dự án Anomaly.

## Repo Layout

- `client/`: shared frontend workspace and toolchain
- `client/mobile/`: consumer app với Tauri 2, React 19, TypeScript, Android và Linux desktop
- `client/desktop/`: admin desktop app với Tauri 2, React 19, TypeScript
- `server/`: backend API Go, worker projection, hạ tầng local bằng Docker Compose
- `specs/`: tài liệu thiết kế nội bộ

## Docs

- `server/README.md`: điểm vào chính cho backend docs
- `client/README.md`: shared frontend workspace commands
- `client/mobile/README.md`: hướng dẫn cho consumer app

## Quick Start

Backend:

```bash
cd server
go test ./...
```

Client:

```bash
cd client
corepack enable
yarn install
yarn dev:mobile
```

Admin desktop:

```bash
cd client
yarn dev:desktop
```

## Hướng dẫn chạy trên máy bạn

### Cấu trúc thư mục nộp

- `server/`, `client/`, `kyc-service/`, `anomaly-service/`, `consumer-mock/`, `deploy/`: mã nguồn
- `database/`: dump 2 database (MongoDB, EventStoreDB) và hướng dẫn restore (`database/README.md`)
- `docs/`, `specs/`: tài liệu kiến trúc và thiết kế
- Thư viện (`node_modules`, `.venv`, `target`, `dist`) đã được xoá để giảm dung lượng; cài lại bằng các lệnh bên dưới

### Yêu cầu

Docker + Docker Compose. Chạy client cần Node 22+ (Yarn qua `corepack`); build app Tauri cần Nix (`nix develop` trong `client/`).

### 1. Cấu hình

```bash
cp server/.env.example server/.env              # nếu chưa có .env
cp anomaly-service/.env.example anomaly-service/.env
cp client/.env.example client/.env
```

Đặt `KAFKA_ADVERTISED_HOST` trong `server/.env` (mặc định `./dev-up.sh` tự lấy IP máy).

### 2. Khôi phục dữ liệu (tuỳ chọn)

Làm theo `database/README.md` để nạp EventStoreDB và MongoDB từ bản dump. Nếu bỏ qua, hệ thống sẽ khởi tạo dữ liệu trống (chạy `make migrate` và `make seed` với `APP_ENV=development`, `SEED_DEMO_ENABLED=true` để có dữ liệu demo).

### 3. Chạy backend bằng Docker

```bash
cd server
./dev-up.sh        # tương đương: docker compose --profile app up --build
```

Bao gồm API, projector, relay, notifier, KYC service, Kafka, EventStoreDB, MongoDB (replica set `rs0`), Redis và RustFS.

| Dịch vụ | Địa chỉ |
|---------|---------|
| API | http://localhost:8080 |
| KYC service | http://localhost:8090 |
| Kafka UI | http://localhost:8082 |
| EventStoreDB UI | http://localhost:2113 |
| RustFS console | http://localhost:9001 |
| MongoDB | localhost:27017 |

Chi tiết API: `server/README.md`. Tài khoản admin demo (`admin.staff`) được tạo bởi seeder, mật khẩu lấy từ `ANOMALY_DEMO_ADMIN_PASSWORD` trong `server/.env`.

### 4. Anomaly service (mô hình GATv2)

```bash
make -C deploy/swarm build-ml    # tạo image anomaly-ml-service:swarm
```

Dockerfile cần repo `GATv2/` nằm cùng cấp với repo này (xem `anomaly-service/README.md`, mục Docker image).

Trọng số mô hình, bundle và ngưỡng đã hiệu chỉnh nằm ngoài mã nguồn (`anomaly-service/models`, `anomaly-service/data`); hãy giữ nguyên các thư mục này. KYC chế độ thật cũng cần model ở `kyc-service/models` (mount qua `KYC_MODEL_DIR`). Khi chưa có, dùng `consumer-mock/` để mô phỏng.

### 5. Client

```bash
cd client
corepack enable && yarn install
yarn dev:mobile     # app khách hàng, http://localhost:1420
yarn dev:desktop    # app admin, http://localhost:1422
```
