# Anomaly Service

Kafka publisher cho model GATv2: nhận trigger `TransferCreated`, đọc event gốc +
lịch sử từ EventStoreDB, chạy inference bằng bundle GATv2 local, publish kết quả
và ghi dữ liệu phục vụ retrain.

## Nguồn request và history

Relay giữ nguyên payload banking khi publish Kafka, bao gồm account/device ID và
`ClientIP`. Kafka được dùng làm trigger; `anomaly-service` đọc `Record.Event` và
lịch sử trước thời điểm request trực tiếp từ EventStoreDB stream `banking`.
Cách này dùng cùng source of truth cho cả request và history. Server hiện chưa có
HTTP history API cho Python; reader có thể thay thế nếu API đó được bổ sung.
Bundle/scaler vẫn là artifact GATv2 đã export, không train lại.

## Luồng

```text
TransferCreated
  → relay publish Kafka (giữ nguyên payload banking)
  → anomaly-service consume TransferCreated
  → đọc raw request + toàn bộ transfer event trước occurredAt từ EventStoreDB
  → GATv2 ReplayPredictor (bundle local)
  → label 1 → BLOCK, label 0 → ALLOW
  → ký HMAC, publish decision vào anomaly.bank.risk-decisions.v1
  → ghi JSONL + publish anomaly.model.training.v1
  → commit Kafka offset

Server consume cùng topic
  → xác minh schema + HMAC
  → lưu model assessment vào risk_decisions
  → commit offset sau khi lưu/park thành công
```

Chỉ commit offset sau khi publish risk **và** ghi training record thành công.
Nếu inference, publish risk hoặc ghi training lỗi, worker dừng trước khi đọc
message tiếp theo và giữ offset để xử lý lại sau restart (at-least-once).
Input không phải trigger được commit; trigger sai định dạng JSON/envelope được
log và bỏ qua có chủ đích. Lỗi model/history, identity không tương thích và lỗi
output không được coi là input có thể bỏ qua.

Output dùng đúng schema `consumer-mock`: `transfer_id`, `source_event_id`,
`decision`, `is_anomalous`, `risk_score`, `model_version`, `evaluated_at`,
`signature`. Worker dùng score/model version thật từ GATv2; `predicted_label=1`
phát `BLOCK`, label `0` phát `ALLOW`. Label đã được kiểm tra bằng
`int(risk_score >= threshold)` theo threshold của bundle. Không sinh `PENDING`
hoặc `ALLOW` thay thế khi inference lỗi: worker giữ offset và dừng để retry.

Server dùng decision này qua luồng `risk_decisions` hiện tại, kiểm tra
`source_event_id` khớp `TransferCreated` của `transfer_id`. `ALLOW` vẫn yêu cầu OTP
hợp lệ để hoàn tất giao dịch; `BLOCK` được xử lý theo luồng fail transfer hiện tại.
Prediction sáu field gốc (kèm threshold/feature version) vẫn giữ trong training
record. Worker không phát thêm message assessment riêng ngoài response trên topic này.

**Thay mock:** dừng process `consumer-mock` trước khi tạo giao dịch mới để test
worker thật. Hai consumer group khác nhau đều nhận cùng event; server giữ terminal
decision nhận đầu tiên, nên không chạy mock và model thật cùng lúc. Các decision
cũ do mock tạo không được worker ghi đè; dùng giao dịch mới để verify.

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
MODEL_BUNDLE_PATH=./models/gatv2-1m-baseline-v1
GATV2_SRC_PATH=../../GATv2/src
MODEL_DEVICE=cpu
```

Nếu `fraud_pipeline` đã được cài trong environment, có thể bỏ `GATV2_SRC_PATH`.
Đường dẫn tương đối được tính từ thư mục chạy lệnh. Khi không đặt
`MODEL_BUNDLE_PATH`, service dùng bundle `models/gatv2-1m-baseline-v1` đi kèm,
được resolve theo vị trí module, không phụ thuộc thư mục chạy lệnh.
Source GATv2 vẫn là dependency ngoài, cần cung cấp theo hướng dẫn trên.

## Cấu hình

Copy `.env.example` and change the values for the environment where the worker
runs. `KAFKA_BROKERS` and `EVENTSTORE_URI` must be reachable from the worker
container or process. The consumer group is shared by all worker replicas.

```env
KAFKA_BROKERS=localhost:9092
KAFKA_INPUT_TOPIC=anomaly.bank.events.v1
KAFKA_OUTPUT_TOPIC=anomaly.bank.risk-decisions.v1
KAFKA_TRAINING_TOPIC=anomaly.model.training.v1
KAFKA_CONSUMER_GROUP=ml-pipeline
KAFKA_AUTO_OFFSET_RESET=earliest
KAFKA_DELIVERY_TIMEOUT_SECONDS=30
KAFKA_RISK_DECISION_SIGNING_KEY=<shared-secret-generated-once>

MODEL_BUNDLE_PATH=./models/gatv2-1m-baseline-v1
GATV2_SRC_PATH=../../GATv2/src
MODEL_DEVICE=cpu

EVENTSTORE_URI=esdb://localhost:2113?tls=false
EVENTSTORE_STREAM=banking

TRAINING_DATA_PATH=./data/training/predictions.jsonl
TRAINING_DATA_FSYNC=true
METRICS_PORT=9100
TRAINING_DATA_MAX_BYTES=104857600
TRAINING_DATA_BACKUP_COUNT=5
```

Khi chạy trong Docker Compose của `server/`, dùng:

```env
KAFKA_BROKERS=kafka:29092
EVENTSTORE_URI=esdb://eventstore:2113?tls=false
```

### Chữ ký HMAC-SHA256

`KAFKA_RISK_DECISION_SIGNING_KEY` là shared secret HMAC, không phải chứng chỉ SSL.
Tạo một lần bằng OpenSSL:

```bash
export KAFKA_RISK_DECISION_SIGNING_KEY="$(openssl rand -hex 32)"
```

Lưu cùng giá trị này vào cấu hình riêng của model và server; không chạy lệnh tạo
key riêng cho từng bên. Trong file `.env`, đặt giá trị hex đã tạo, không đặt nguyên
chuỗi `openssl rand -hex 32` hoặc shell substitution (dotenv không chạy lệnh shell).
Không commit secret. Worker từ chối khởi động khi biến thiếu hoặc chỉ có khoảng trắng.
Key được dùng nguyên văn dưới dạng UTF-8, không decode hex, tương tự server Go.

Publisher thêm `signature` dạng hex chữ thường vào mỗi risk message:

```text
signature = HMAC-SHA256(secret_key_utf8, canonical_json_without_signature_utf8)
```

Canonical JSON theo đúng struct `riskDecisionUnsigned` của server Go và mock,
gồm bảy field đúng thứ tự: `transfer_id`, `source_event_id`, `decision`,
`is_anomalous`, `risk_score`, `model_version`, `evaluated_at`; bỏ `signature`.
JSON compact UTF-8, escape HTML (`<`, `>`, `&`) và U+2028/U+2029 như `encoding/json`;
số thực theo cách biểu diễn của Go (ví dụ `0.0 → 0`, `1e-07 → 1e-7`), thời gian UTC
RFC3339Nano bỏ zero thừa. Không sắp xếp key theo alphabet. Server xác minh HMAC
bằng constant-time comparison, kiểm tra metadata/correlation và decision hợp lệ
trước khi lưu. Message unsigned, sai key, bị sửa hoặc sai schema được park vào
`risk_decision_rejections`; lỗi lưu DB không commit offset.

Các test dùng chung fixture Python/Go để kiểm tra bytes canonical và server decoder
của cả decision và assessment. Nhánh assessment hiện có của server vẫn được bảo vệ
HMAC và kiểm tra thiếu/null field, nhưng worker thật hiện phát decision theo mock.
Training record/topic giữ prediction gốc, không có chữ ký decision.
HMAC không mã hóa payload và không tự chống replay; DB xử lý redelivery.

## Chạy

```bash
cd anomaly-service
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

`METRICS_PORT` đã có trong config và các metric dưới đây đã được khai báo.
Entry point hiện chưa khởi động HTTP exporter; không dùng `/metrics` để verify
luồng decision này:

| Metric | Meaning |
| --- | --- |
| `ml_messages_total{result}` | Received messages classified as `predicted`, `ignored`, or `failed` |
| `ml_predictions_total` | Successfully published predictions |
| `ml_processing_seconds` | Message processing duration histogram |
| `ml_last_success_timestamp_seconds` | UTC Unix timestamp of the last committed message |

The Swarm autoscaler uses Kafka consumer lag as its primary signal. Prometheus
and the Kafka exporter collect that lag; CPU is not used as the scaling trigger.

Yêu cầu: Kafka, EventStoreDB và bundle GATv2 đã sẵn sàng. Không cần model HTTP
service.

Nếu chỉ chạy CPU và muốn tránh tải CUDA, cài torch từ CPU index trước:

```bash
pip install --index-url https://download.pytorch.org/whl/cpu torch
pip install -e '.[dev]'
```

## Docker image

The Dockerfile expects this layout in the build context parent:

```text
git/
├── anomaly/anomaly-service/
└── GATv2/src/fraud_pipeline/
```

Build from the `git/` directory so the image contains the matching GATv2
source. `network.host` is required in this environment because Docker's bridge
DNS cannot resolve PyPI:

```bash
cd deploy/swarm
make build-ml
```

The image installs the CPU PyTorch wheel and includes the local model bundle.
For GPU inference, use a CUDA base image and a separately tested dependency
set; do not change `MODEL_DEVICE` alone.

## Docker Swarm deployment

The ML service is deployed separately from the local API Compose stack. It does
not expose a prediction HTTP endpoint; Kafka is its input and output interface.
The stack contains:

- `ml-service`: Kafka consumer and GATv2 inference worker
- `kafka-exporter`: exposes consumer-group lag
- `prometheus`: scrapes worker metrics and lag
- `ml-autoscaler`: calls the Docker API to change worker replicas

Build and deploy on a Swarm manager:

```bash
cd deploy/swarm
make build-ml
make build-autoscaler

export ML_KAFKA_BROKERS='<broker-ip>:9092'
export ML_KAFKA_EXPORTER_SERVER='<broker-ip>:9092'
export ML_EVENTSTORE_URI='esdb://<eventstore-ip>:2113?tls=false'
# Export KAFKA_RISK_DECISION_SIGNING_KEY using the existing shared secret first.
docker stack deploy -c ml-stack.yml ml
make services
```

The default autoscaling bounds are one to four replicas:

```bash
ML_MIN_REPLICAS=1
ML_MAX_REPLICAS=4
ML_SCALE_UP_LAG=1000
ML_SCALE_DOWN_LAG=100
AUTOSCALER_COOLDOWN_SECONDS=120
```

Set the input Kafka topic partitions to at least `ML_MAX_REPLICAS`. Kafka can
assign only one consumer in a group to each partition, so extra replicas remain
idle. The autoscaler mounts `/var/run/docker.sock` and must run on a manager
node; treat that socket as full Docker control access.

Manual scaling and logs:

```bash
make scale-up SCALE_UP_TO=3
make scale-down SCALE_DOWN_TO=1
make logs-autoscaler
```

## Verification

Chạy static/unit/contract tests từ root repo bằng Nix:

```bash
nix develop ./server
(cd anomaly-service && python -m pytest -q && ruff check app tests scripts)
(cd server && go test ./cmd/api ./internal/domain/riskassessment ./internal/infrastructure/kafka -count=1)
```

Test Go dùng payload ký bởi Python để xác minh canonical JSON/HMAC, kiểm tra
decision Python đi vào decision store, assessment chỉ ghi assessment store; unsigned/tampered
bị park và lỗi storage được trả về để không commit offset. Các test này chạy
không cần Kafka/Mongo, không chứng minh inference hoặc DB/network thật.

### Verify Kafka → server → Mongo bằng message tổng hợp

Cần API code mới đã restart/rebuild, Kafka/Mongo đang chạy, cùng signing key và
`KAFKA_OUTPUT_TOPIC` (worker) bằng `KAFKA_RISK_DECISION_TOPIC` (server). Topic mặc
định của cả hai là `anomaly.bank.risk-decisions.v1`. Khi chạy Compose, có thể
rebuild API bằng lệnh sau từ root repo (sẽ khởi động dependencies nếu cần):

```bash
docker compose --env-file server/.env -f server/docker-compose.yml --profile app up -d --build anomaly-server
```

Trong Nix shell, nạp key server hiện có (không tạo key mới riêng cho test):

```bash
set -a
source server/.env
set +a
python anomaly-service/scripts/build_verify_messages.py --output-dir /tmp/anomaly-decision-verify
```

Script tạo hai transfer ID tổng hợp: `valid.json` là BLOCK, `allow.json` là ALLOW,
và bốn message lỗi (`tampered`, `unsigned`, `wrong-key`, `missing-score`); không ghi secret ra file
hay stdout. Đây là test contract/transport với score tổng hợp, không chạy model
và không tạo giao dịch/chuyển tiền.

Publish BLOCK hợp lệ hai lần để kiểm tra idempotency, rồi publish ALLOW cho ID khác:

```bash
for attempt in 1 2; do
  docker compose --env-file server/.env -f server/docker-compose.yml exec -T kafka \
    /opt/kafka/bin/kafka-console-producer.sh \
    --bootstrap-server kafka:29092 \
    --topic "${KAFKA_RISK_DECISION_TOPIC:-anomaly.bank.risk-decisions.v1}" \
    < /tmp/anomaly-decision-verify/valid.json
done
docker compose --env-file server/.env -f server/docker-compose.yml exec -T kafka \
  /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server kafka:29092 \
  --topic "${KAFKA_RISK_DECISION_TOPIC:-anomaly.bank.risk-decisions.v1}" \
  < /tmp/anomaly-decision-verify/allow.json
```

Publish các trường hợp reject:

```bash
for variant in tampered unsigned wrong-key missing-score; do
  docker compose --env-file server/.env -f server/docker-compose.yml exec -T kafka \
    /opt/kafka/bin/kafka-console-producer.sh \
    --bootstrap-server kafka:29092 \
    --topic "${KAFKA_RISK_DECISION_TOPIC:-anomaly.bank.risk-decisions.v1}" \
    < "/tmp/anomaly-decision-verify/$variant.json"
done
docker compose --env-file server/.env -f server/docker-compose.yml logs --since 5m anomaly-server
```

Kỳ vọng message hợp lệ có log `risk decision processed`, với decision BLOCK hoặc
ALLOW tương ứng; bốn message lỗi có log `invalid risk decision parked`.
Lấy hai transfer ID do script in ra, rồi chạy các truy vấn tương đương SELECT
trong Mongo shell (thay placeholder; các lệnh dưới chỉ đọc):

```javascript
use anomaly
db.risk_decisions.find({transfer_id: {$in: ["<block-transfer-id>", "<allow-transfer-id>"]}})
db.risk_decisions.countDocuments({transfer_id: "<block-transfer-id>"}) // 1 dù publish hai lần
db.risk_decision_rejections.find({created_at: {$gte: new Date(Date.now() - 5*60*1000)}}, {reason: 1, topic: 1, partition: 1, offset: 1})
```

### Verify model thật

Dừng `consumer-mock`. Chạy worker với dependencies Python/GATv2 và bundle tương thích,
đợi worker nhận partition rồi tạo transfer mới trên app. Kiểm tra log
`decision published`, `risk decision processed`, và
JSONL `request.payload.ip_address` (khi server quan sát được IP). Bundle hiện tại
không có feature IP: IP được giữ cho training/audit, không đưa vào input bundle.
History đưa vào model cũng bỏ `riskAssessment` để tránh feedback/leakage.
Kiểm tra `risk_decisions` có ID giao dịch mới và đúng decision. Khi confirm, ALLOW
vẫn yêu cầu OTP; BLOCK phải fail giao dịch và không debit/credit. Nếu chưa có
decision do model đang lỗi hoặc chưa xử lý, server trả `RISK_CHECK_PENDING`.

Cần dùng source `GATv2/src` có capability `distinct-account-nodes-v1` và restart
worker (hoặc rebuild ML image vì Dockerfile copy source GATv2 lúc build). Source
mới giữ mỗi ObjectID account thành node riêng và hỗ trợ request chưa có history,
vẫn dùng bundle/scaler/threshold hiện tại. Source cũ thiếu capability sẽ tiếp tục
bị guard từ chối account ngoài catalog; không bỏ guard để gộp mọi account lạ vào
`__unknown__`. Test Kafka tổng hợp không phải bằng chứng model chạy trên dữ liệu thật.

### Test weights thật với account mới, không cần Kafka/DB

Từ root repo Anomaly, dùng Nix để chạy test opt-in (cần source GATv2 đã cập nhật):

```bash
nix develop --impure --expr 'let f = builtins.getFlake (toString ./server); p = f.inputs.nixpkgs.legacyPackages.x86_64-linux; in p.mkShell { packages = [ (p.python3.withPackages (ps: with ps; [ pytest numpy torch torch-geometric ])) ]; }' --command bash -c 'cd anomaly-service && PYTHONDONTWRITEBYTECODE=1 RUN_GATV2_BUNDLE_TEST=1 python -m pytest -q tests/test_real_bundle_accounts.py'
```

Test dùng bundle đi kèm, hai ObjectID tổng hợp và history rỗng; kiểm tra node nguồn/đích
khác nhau và khác unknown, inference trả score hữu hạn, IP vẫn giữ trong training
request. Không train lại, không publish Kafka, không ghi DB và không thay decision
của giao dịch thật. Đây là bằng chứng chạy inference, chưa phải đánh giá chất lượng
phát hiện gian lận hay bằng chứng Kafka E2E.

## Output Kafka

Topic: `anomaly.bank.risk-decisions.v1` (cấu hình được).

```json
{
  "transfer_id": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
  "source_event_id": "<TransferCreated-event-id-from-relay>",
  "decision": "BLOCK",
  "is_anomalous": true,
  "risk_score": 0.9821,
  "model_version": "gatv2-device-control-v1",
  "evaluated_at": "2026-10-19T10:30:01.123Z",
  "signature": "<64-lowercase-hex-HMAC-SHA256-characters>"
}
```

- Kafka key: `transfer_id`, giống `consumer-mock`.
- `source_event_id` giữ nguyên ID trigger relay, để server kiểm tra correlation.
- `risk_score` là sigmoid của weighted-BCE logit, **không phải xác suất gian
  lận đã hiệu chỉnh**.
- Policy đã chốt: `predicted_label=1 → BLOCK/true`, `0 → ALLOW/false`.
- `threshold`, `predicted_label`, `feature_version` vẫn có trong training record;
  không thêm field vào envelope decision của mock.
- Server dedupe theo `transfer_id`, không ghi đè terminal decision đã chấp nhận.

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
- `event.occurred_at` lấy từ raw request `occurredAt`;
  timestamp Kafka do relay gán chỉ dùng để kiểm tra envelope đầu vào.
- JSONL dùng POSIX file lock khi repair/rotate/append. File mới và file đang mở
  được giới hạn quyền `0600`; thư mục training mới dùng `0700`. Với thư mục đã
  tồn tại, operator cần kiểm tra quyền thư mục và các backup cũ riêng.
- Nếu lần ghi trước bị ngắt, fragment JSON cuối file được truncate trước khi
  ghi tiếp; record JSON hoàn chỉnh thiếu newline được giữ nguyên và thêm newline.
- File active có giới hạn mặc định 100 MiB, giữ năm backup `.1` đến `.5`.
  Khi rotation, backup cũ nhất bị thay thế. Archive sang kho riêng nếu cần giữ
  training data lâu dài; thay đổi giới hạn bằng `TRAINING_DATA_MAX_BYTES` và
  `TRAINING_DATA_BACKUP_COUNT`. Record lớn hơn giới hạn làm worker dừng, không
  commit offset. Cần giám sát dung lượng đĩa; rotation không bảo đảm đĩa còn trống.
- Các writer dùng chung path phải dùng filesystem hỗ trợ POSIX `flock`.
  Khi chạy replica trên nhiều node với volume local, mỗi node có file training
  riêng; training topic cần retention và quyền truy cập được cấu hình riêng.
- `event` và `request` lấy từ `TransferCreated` trong EventStoreDB; request được
  bổ sung device/auth/IP từ snapshot `transfer` cùng event. `ip_address` chỉ có
  khi `ClientIP` có giá trị; không bịa IP cho dữ liệu cũ.
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
- Kafka relay enrich `model_features` cho trigger; service này không dùng
  `model_features`, GATv2 tự tính từ history.
- Chưa có DLQ. Trigger sai định dạng được log và commit; lỗi xử lý/model/history
  giữ offset và dừng worker, cần restart/giám sát. Lỗi xử lý vĩnh viễn cần được
  operator khắc phục trước khi retry, tránh restart loop.
- Output decision và HMAC bám theo consumer Go; cần cùng signing key/topic ở
  hai bên. Vẫn cần xác minh Kafka E2E sau khi cập nhật source serving GATv2.
- Chưa benchmark latency online và chưa kiểm chứng trên dữ liệu thật.
- `TRAINING_DATA_PATH` và topic training chứa account/device ID gốc (không bị
  pseudonymize) vì cần cho graph GATv2. Cần giới hạn quyền đọc, retention và mã
  hóa phù hợp; risk topic thì chỉ chứa transaction ID và kết quả model.
- Lần chạy đầu với `KAFKA_AUTO_OFFSET_RESET=earliest` sẽ replay toàn bộ topic
  input. Nếu topic đã có nhiều dữ liệu, nên đặt `latest` hoặc xóa consumer group
  trước khi chạy.
- **Account identity:** input model có `sourceAccountId`/`destinationAccountId`.
  Predictor GATv2 có capability `distinct-account-nodes-v1` dựng node account mới
  theo request + history đã validate, mỗi ID một node; không đổi ObjectID thành
  UUID training, không gộp account mới vào `__unknown__`. Bundle và scaler giữ
  nguyên. Catalog runtime không sửa `nodes.json`; graph/dynamic features vẫn dùng
  strict prior history và snapshot cutoff hiện tại. Account mới được giữ type
  account để score cold-start, không suy diễn ngày mở tài khoản từ request.
  Source cũ chưa có capability vẫn bị guard từ chối để tránh mất identity.
  Test compatibility/cold-start không chứng minh chất lượng hay calibration trên
  dữ liệu thật; vẫn cần đánh giá trước khi vận hành thực tế.
- Cấu hình local hiện dùng Kafka PLAINTEXT và EventStoreDB không TLS. Không dùng
  cấu hình này qua network không tin cậy; TLS/auth/ACL, mã hóa storage và retention
  training topic phải được chuẩn bị cho môi trường triển khai.
