# Kafka Consumer Mock

Consumer độc lập tối giản cho Kafka. Service này đọc `TransferCreated` envelope
từ request topic, chọn các field transaction và `model_features` do relay enrich,
log JSON ra stdout rồi publish risk decision sang response topic. Không phụ thuộc
vào `server/`; model pipeline thật sẽ được triển khai riêng sau này.

## Configuration

```env
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=anomaly.bank.events.v1
KAFKA_OUTPUT_TOPIC=anomaly.bank.risk-decisions.v1
KAFKA_CONSUMER_GROUP=model-pipeline
MOCK_RISK_DECISION=ALLOW # PENDING, ALLOW, or BLOCK
MOCK_MODEL_VERSION=mock-v1
```

Khi chạy cùng server Docker Compose, consumer chạy ngoài Docker nên dùng broker
host là `localhost:9092`.

## Run locally

```bash
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
{"event_id":"test-1","event_type":"TransferCreated","stream":"banking","stream_revision":1,"schema_version":2,"payload":{"transfer":{"ID":"TX001","Type":"transfer","Source":{"AccountNo":"ACC001"},"Destination":{"AccountNo":"ACC002"},"Amount":50000000,"Channel":"mobile","CreatedAt":"2026-10-17T00:50:00Z","DeviceID":"DEVICE01","Platform":"android","Authentication":{"Method":"email_otp","Verified":false}}},"model_features":{"avg_amount_7d":12500000,"tx_count_1h":3,"tx_count_24h":12,"last_tx_at":"2026-10-16T00:50:00Z","new_receiver":true,"new_device":false}}
```

Consumer chỉ xử lý `TransferCreated`, log model input và publish risk decision
sang `KAFKA_OUTPUT_TOPIC`. Đặt `MOCK_RISK_DECISION=BLOCK` để mô phỏng giao dịch
bất thường. Đặt `MOCK_RISK_DECISION=PENDING` để mô phỏng model chưa có kết quả.
Các event khác sẽ được ACK và bỏ qua.

Consumer commit offset sau khi log thành công. EventStoreDB relay thuộc
`server/cmd/relay`, không thuộc service consumer này.
