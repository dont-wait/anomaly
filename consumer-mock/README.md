# Kafka Consumer Mock

Consumer độc lập tối giản cho Kafka. Service này đọc event envelope từ topic,
tự chọn các field transaction cần cho model và log JSON ra stdout, không phụ
thuộc vào `server/`. Thư mục này chỉ là mock consumer để kiểm thử luồng Kafka;
model pipeline thật sẽ được triển khai riêng sau này.

## Configuration

```env
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=anomaly.bank.events.v1
KAFKA_CONSUMER_GROUP=model-pipeline
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
{"event_id":"test-1","event_type":"TestEvent","stream":"banking","stream_revision":1,"schema_version":1,"payload":{"message":"hello"}}
```

Consumer commit offset sau khi log thành công. EventStoreDB relay thuộc
`server/cmd/relay`, không thuộc service consumer này.
