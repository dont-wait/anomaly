# Database dump

| File | Nội dung |
|------|----------|
| `mongo/anomaly.archive.gz` | MongoDB 7.0, database `anomaly` (mongodump `--gzip --archive`) |
| `eventstore/eventstore-data.tar.gz` | Dữ liệu EventStoreDB 23.10.8 (copy thư mục `/var/lib/eventstore` khi đã dừng) |

EventStoreDB là nguồn dữ liệu gốc (event sourcing); MongoDB chứa projection có thể dựng lại.

## Restore (chạy trong `server/`, đã có `.env`)

```bash
# 1. EventStoreDB: nạp dữ liệu vào volume trước khi khởi động
docker volume create server_eventstore-data
docker run --rm -v server_eventstore-data:/data -v "$PWD/../database/eventstore":/in alpine \
  tar xzf /in/eventstore-data.tar.gz -C /data

# 2. MongoDB (replica set rs0)
docker compose up -d mongo      # chờ healthy
docker cp ../database/mongo/anomaly.archive.gz anomaly-mongo:/tmp/
docker exec anomaly-mongo mongorestore -u "$MONGO_ROOT_USERNAME" -p "$MONGO_ROOT_PASSWORD" \
  --authSource admin --gzip --archive=/tmp/anomaly.archive.gz --drop

# 3. Chạy toàn bộ hệ thống
docker compose --profile app up -d --build
```

Nếu không restore, có thể tạo dữ liệu mới bằng `make migrate` và `make seed` (xem README gốc).
