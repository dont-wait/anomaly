# Anomaly ML Pipeline

Kafka publisher cho model GATv2: nhận trigger `TransferCreated`, đọc event gốc +
lịch sử từ EventStoreDB, chạy inference bằng bundle GATv2 local, publish kết quả
và ghi dữ liệu phục vụ retrain.

## Vì sao không dùng payload Kafka để inference

Relay của `server/` pseudonymize `accountId`, `accountNo`, `deviceId` trước khi
publish Kafka. GATv2 dựng graph bằng identity tài khoản, nên payload Kafka không
khớp node catalog đã train. Vì vậy:

- Kafka chỉ dùng làm **trigger** (`event_id`, `transaction_id`, `occurred_at`).
- `ml-pipeline` đọc **Record.Event gốc + history** trực tiếp từ EventStoreDB
  stream `banking`.
- Bundle và scaler vẫn là artifact GATv2 đã export; không train lại.

Server hiện chưa có HTTP API trả history cho Python. EventStoreDB đã là source of
truth và có đủ event, nên service này dùng `esdbclient` trực tiếp. Nếu sau này
server thêm history API/projection, chỉ cần thay `EventStoreHistory`.

## Luồng

```text
TransferCreated
  → relay publish Kafka (payload đã redact/pseudonymize)
  → ml-pipeline consume TransferCreated
  → đọc raw request + toàn bộ transfer event trước occurredAt từ EventStoreDB
  → GATv2 ReplayPredictor (bundle local)
  → publish anomaly.bank.risk.v1
  → ghi JSONL + publish anomaly.model.training.v1
  → commit Kafka offset
```

Chỉ commit offset sau khi publish risk **và** ghi training record thành công.
Nếu một bước lỗi, message sẽ được xử lý lại sau restart (at-least-once).

Output `event_id` là ID của `TransferCreated`; `transaction_id` dùng để server
correlate với transfer. Output được publish trước khi worker commit offset, nên
server có thể nhận response trước khi offset commit; hãy xử lý idempotent theo
`transaction_id`.

## Model bundle

`MODEL_BUNDLE_PATH` trỏ tới thư mục bundle do GATv2 export, gồm:

```text
bundle.json
model.pt
manifest.json
preprocessing.json
node_preprocessing.json
nodes.json
selection.json
```

Service dùng đúng `ReplayPredictor` của GATv2. Cần cài dependencies của GATv2
(`torch`, `numpy`) và cung cấp source path:

```env
MODEL_BUNDLE_PATH=../../../GATv2/outputs/registry/gatv2-device-control-v1
GATV2_SRC_PATH=../../../GATv2/src
MODEL_DEVICE=cpu
```

Nếu `fraud_pipeline` đã được cài trong environment, có thể bỏ `GATV2_SRC_PATH`.
Đường dẫn tương đối được tính từ thư mục chạy lệnh.

## Cấu hình

```env
KAFKA_BROKERS=localhost:9092
KAFKA_INPUT_TOPIC=anomaly.bank.events.v1
KAFKA_OUTPUT_TOPIC=anomaly.bank.risk.v1
KAFKA_TRAINING_TOPIC=anomaly.model.training.v1
KAFKA_CONSUMER_GROUP=ml-pipeline
KAFKA_AUTO_OFFSET_RESET=earliest
KAFKA_DELIVERY_TIMEOUT_SECONDS=30

MODEL_BUNDLE_PATH=../../../GATv2/outputs/registry/gatv2-device-control-v1
GATV2_SRC_PATH=../../../GATv2/src
MODEL_DEVICE=cpu

EVENTSTORE_URI=esdb://localhost:2113?tls=false
EVENTSTORE_STREAM=banking

TRAINING_DATA_PATH=./data/training/predictions.jsonl
TRAINING_DATA_FSYNC=true
```

Khi chạy trong Docker Compose của `server/`, dùng:

```env
KAFKA_BROKERS=kafka:29092
EVENTSTORE_URI=esdb://eventstore:2113?tls=false
```

## Chạy

```bash
cd server/ml-pipeline
cp .env.example .env
python -m venv .venv
. .venv/bin/activate
pip install -e '.[dev]'
anomaly-ml-pipeline
```

`python -m app` và console script tự load `.env` trong thư mục hiện tại; biến môi
trường đã export sẽ được ưu tiên hơn file.

Hoặc:

```bash
python -m app
```

Yêu cầu: Kafka, EventStoreDB và bundle GATv2 đã sẵn sàng. Không cần model HTTP
service.

Nếu chỉ chạy CPU và muốn tránh tải CUDA, cài torch từ CPU index trước:

```bash
pip install --index-url https://download.pytorch.org/whl/cpu torch
pip install -e '.[dev]'
```

## Output Kafka

Topic: `anomaly.bank.risk.v1` (cấu hình được).

```json
{
  "schema_version": 1,
  "event_id": "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70",
  "transaction_id": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
  "occurred_at": "2026-10-19T10:30:00.123456789Z",
  "scored_at": "2026-10-19T10:30:01.123Z",
  "risk_score": 0.9821,
  "predicted_label": 1,
  "threshold": 0.9517361521720886,
  "model_version": "gatv2-device-control-v1",
  "feature_version": "schema-3-0b9efd44d793"
}
```

- Key: `transaction_id:model_version:feature_version`; coi key là opaque, không
  split để lấy field.
- `risk_score` là sigmoid của weighted-BCE logit, **không phải xác suất gian
  lận đã hiệu chỉnh**.
- `predicted_label` chỉ là kết quả sau ngưỡng, không phải quyết định
  allow/review/block của backend.
- Backend nên upsert theo key; duplicate có thể xảy ra khi retry.

## Training data

Ghi JSONL tại `TRAINING_DATA_PATH` và publish cùng record lên
`KAFKA_TRAINING_TOPIC`.

```json
{
  "schema_version": 1,
  "recorded_at": "2026-10-19T10:30:01.123Z",
  "record_id": "sha256...",
  "event": {
    "event_id": "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70",
    "event_type": "TransferCreated",
    "stream": "banking",
    "stream_revision": 123,
    "occurred_at": "2026-10-19T10:30:00.123456789Z",
    "schema_version": 2
  },
  "request": {
    "eventId": "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70",
    "eventType": "TransferCreated",
    "occurredAt": "2026-10-19T10:30:00.123456789Z",
    "payload": {"...": "raw TransferCreated event from EventStoreDB"}
  },
  "response": {
    "transaction_id": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
    "risk_score": 0.9821,
    "predicted_label": 1,
    "threshold": 0.9517361521720886,
    "model_version": "gatv2-device-control-v1",
    "feature_version": "schema-3-0b9efd44d793"
  },
  "prediction_key": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1:gatv2-device-control-v1:schema-3-0b9efd44d793"
}
```

- `record_id` ổn định theo event + request + response, dùng để dedupe khi retry.
- `event` và `request` đều là raw `TransferCreated` đọc từ EventStoreDB, không
  phải payload Kafka đã redact.
- Record **không phải label**. Muốn train supervised, cần join với outcome sau
  này (`TransferCompleted`, `TransferCancelled`, điều tra/chargeback...) theo
  `transaction_id`, và chỉ dùng thông tin có sau `occurred_at` làm label.
- Muốn tái tạo feature GATv2, replay lịch sử EventStoreDB trước `occurred_at`;
  không dùng lại `response` làm feature (leakage).

## Giới hạn hiện tại

- Mỗi request đọc toàn bộ stream `banking` để lấy request + prefix. Đúng nhưng
  chưa tối ưu throughput; có thể thay bằng đọc incremental/cache sau.
- Chỉ score `TransferCreated`. Nếu muốn score lúc `TransferCompleted`, cần model
  và feature policy train theo thời điểm đó; không dùng lại model request-time.
- Kafka relay chỉ enrich `model_features` cho `TransferCompleted`; service này
  không dùng `model_features`, GATv2 tự tính từ history.
- Chưa có DLQ. Lỗi input/model/history làm worker dừng và không commit; cần
  restart/giám sát.
- Chưa có consumer Go cho topic risk; backend/FE cần được nối riêng.
- Chưa benchmark latency online và chưa kiểm chứng trên dữ liệu thật.
- `TRAINING_DATA_PATH` và topic training chứa account/device ID gốc (không bị
  pseudonymize) vì cần cho graph GATv2. Cần giới hạn quyền đọc, retention và mã
  hóa phù hợp; risk topic thì chỉ chứa transaction ID và kết quả model.
- Lần chạy đầu với `KAFKA_AUTO_OFFSET_RESET=earliest` sẽ replay toàn bộ topic
  input. Nếu topic đã có nhiều dữ liệu, nên đặt `latest` hoặc xóa consumer group
  trước khi chạy.
- **Bundle compatibility:** `nodes.json` của bundle GATv2 hiện tại chứa account ID
  dạng UUID v5 từ dataset training. EventStoreDB của Anomaly dùng MongoDB
  ObjectID hex cho `sourceAccountId`/`destinationAccountId`, nên các node sẽ
  fallback về `__unknown__` nếu bundle chưa được train/export với đúng ID của
  Anomaly. Cần retrain hoặc thống nhất mapping ID trước khi dùng production.
