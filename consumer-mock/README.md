# Kafka Consumer Mock

Consumer độc lập tối giản cho Kafka, mô phỏng model service thật. Service đọc
`TransferCreated` envelope từ request topic, chọn các field transaction và
`model_features` do relay enrich, log input rồi publish model response v1 vào
cùng output topic mà model service thật sử dụng. Server lưu response vào
collection `risk_assessments`.

## Configuration

Các biến được liệt kê đầy đủ trong [`.env.example`](.env.example).

```env
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=anomaly.bank.events.v1
KAFKA_OUTPUT_TOPIC=anomaly.bank.risk-decisions.v1
KAFKA_CONSUMER_GROUP=model-pipeline
KAFKA_RISK_DECISION_SIGNING_KEY=replace-with-the-server-shared-key
MOCK_RISK_SCORE=0.9821
MOCK_THRESHOLD=0.9517361521720886
MOCK_MODEL_VERSION=gatv2-device-control-v1
MOCK_FEATURE_VERSION=schema-3-0b9efd44d793
```

Khi chạy cùng server Docker Compose, consumer chạy ngoài Docker nên dùng broker
host là `localhost:9092`.

## Run locally

```bash
cp .env.example .env
# Chỉnh .env nếu cần. Chương trình không tự đọc .env; export trước khi chạy.
set -a
source .env
set +a
go run .
```

## Test message

Kafka phải đang chạy từ `server/`. Từ thư mục `server/`, mở Kafka CLI:

```bash
docker compose exec kafka \
  /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server kafka:29092 \
   --topic anomaly.bank.events.v1
```

Nhập message, ví dụ:

```json
{
  "event_id": "test-1",
  "event_type": "TransferCreated",
  "occurred_at": "2026-10-17T00:50:00Z",
  "stream": "banking",
  "stream_revision": 1,
  "schema_version": 2,
  "payload": {
    "transfer": {
      "ID": "TX001",
      "Type": "transfer",
      "Source": {
        "AccountNo": "ACC001"
      },
      "Destination": {
        "AccountNo": "ACC002"
      },
      "Amount": 50000000,
      "Channel": "mobile",
      "CreatedAt": "2026-10-17T00:50:00Z",
      "DeviceID": "DEVICE01",
      "ClientIP": "203.0.113.10",
      "Platform": "android",
      "Authentication": {
        "Method": "email_otp",
        "Verified": false
      }
    }
  },
  "model_features": {
    "avg_amount_7d": 12500000,
    "tx_count_1h": 3,
    "tx_count_24h": 12,
    "last_tx_at": "2026-10-16T00:50:00Z",
    "new_receiver": true,
    "new_device": false
  }
}
```

Consumer chỉ xử lý `TransferCreated`, log model input và publish model response
v1 sang `KAFKA_OUTPUT_TOPIC`. `predicted_label` được tính từ
`risk_score >= threshold`; thay đổi `MOCK_RISK_SCORE` để mô phỏng assessment
normal hoặc anomalous.

Output có cùng schema với model service thật:

```json
{
  "schema_version": 1,
  "event_id": "event-123",
  "transaction_id": "TX001",
  "occurred_at": "2026-10-19T10:30:00.123456789Z",
  "scored_at": "2026-10-19T10:30:01.123Z",
  "risk_score": 0.9821,
  "predicted_label": 1,
  "threshold": 0.9517361521720886,
  "model_version": "gatv2-device-control-v1",
  "feature_version": "schema-3-0b9efd44d793",
  "signature": "hmac-sha256-hex"
}
```

Kafka message key là:

```text
transaction_id:model_version:feature_version
```

Output này là model assessment, không phải quyết định `ALLOW/BLOCK`.
Các event khác sẽ được ACK và bỏ qua.

Consumer commit offset sau khi publish risk decision thành công. EventStoreDB relay thuộc
`server/cmd/relay`, không thuộc service consumer này.
