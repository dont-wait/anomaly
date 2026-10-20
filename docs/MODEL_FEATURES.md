# Model Feature Enrichment

## Mục tiêu

Producer/relay cần gửi thêm các feature phục vụ model trong **cùng event** và
cùng Kafka topic hiện tại:

```text
anomaly.bank.events.v1
```

Các feature cần gửi:

- `avg_amount_7d`: số tiền giao dịch trung bình trong 7 ngày
- `tx_count_1h`: số giao dịch trong 1 giờ gần nhất
- `tx_count_24h`: số giao dịch trong 24 giờ gần nhất
- `last_tx_at`: timestamp của giao dịch trước đó
- `new_receiver`: người nhận có phải người nhận mới không
- `new_device`: thiết bị có phải thiết bị mới không

`last_tx_at` dùng timestamp RFC3339, thống nhất với `occurred_at` trong event
envelope. Nếu chưa có giao dịch trước đó thì giá trị là `null`.

## Vấn đề cần tránh

Không nên xử lý mỗi giao dịch bằng cách query toàn bộ lịch sử rồi tính lại:

```text
query toàn bộ lịch sử
→ tính average 7 ngày
→ đếm giao dịch 1 giờ/24 giờ
→ tìm giao dịch gần nhất
```

Cách này làm độ trễ và chi phí tăng theo số lượng lịch sử của tài khoản.

## Ý tưởng chính

Dùng **incremental materialized feature view**. Producer duy trì một bản tóm
tắt trạng thái theo từng tài khoản nguồn. Mỗi giao dịch chỉ cần đọc một state,
tính feature từ state đó, publish event rồi cập nhật state.

```text
EventStoreDB
    ↓
Relay + Feature Enricher
    ↓
Kafka: anomaly.bank.events.v1
    ↓
Model consumer
```

Feature phải được tính từ trạng thái **trước giao dịch hiện tại**. Sau khi
Kafka xác nhận publish thành công, giao dịch hiện tại mới được đưa vào state.
Điều này tránh việc dữ liệu của giao dịch hiện tại làm sai feature đầu vào cho
chính nó.

## Storage

MongoDB là lựa chọn phù hợp vì hệ thống đã sử dụng MongoDB cho projection và
checkpoint. Không tạo Kafka topic mới cho feature state.

### Collection `model_feature_states`

Mỗi tài khoản nguồn có một document summary:

```json
{
  "_id": "source-account-id",
  "last_tx_at": "2026-10-19T10:15:00Z",
  "amount_sum_7d": 120000000,
  "tx_count_7d": 18,
  "rolling_buckets": [
    {
      "start_at": "2026-10-19T10:00:00Z",
      "amount_sum": 25000000,
      "tx_count": 2
    }
  ],
  "state_version": 42,
  "updated_at": "2026-10-19T10:30:00Z"
}
```

`rolling_buckets` dùng để loại bỏ dữ liệu đã quá 7 ngày mà không cần quét
transaction history. Mỗi bucket giữ thêm các sample timestamp trong rolling
window để xử lý chính xác cutoff theo giây và bỏ qua hoạt động xảy ra sau một
event historical đang được replay. State vẫn bị giới hạn theo rolling window,
không phải toàn bộ transaction history.

### Receiver và device history

Không nên lưu danh sách receiver/device không giới hạn trong một document. Có
thể dùng hai collection có compound unique index:

```text
model_known_receivers(account_id, receiver_account_id)
model_known_devices(account_id, device_id)
```

Producer kiểm tra sự tồn tại tại thời điểm `occurred_at` trước khi cập nhật:

- Không tồn tại receiver → `new_receiver: true`
- Không tồn tại device → `new_device: true`
- Sau khi publish thành công, thêm receiver/device vào history

Đây vẫn là point lookup theo index, không phải scan lịch sử giao dịch.

## Kafka event contract

Giữ cấu trúc payload banking cần thiết và thêm `model_features` ở cấp envelope.
Payload trước khi publish được redact các profile/KYC/contact/media/credential
fields; account number, account ID và device ID được pseudonymize bằng
`KAFKA_PAYLOAD_REDACTION_KEY` (hoặc SHA-256 khi chạy local không cấu hình key).
Consumer mock không log raw payload.

```json
{
  "event_id": "event-123",
  "event_type": "TransferCompleted",
  "stream": "banking",
  "stream_revision": 123,
  "occurred_at": "2026-10-19T10:30:00Z",
  "schema_version": 2,
  "payload": {
    "...": "banking event gốc"
  },
  "model_features": {
    "avg_amount_7d": 6666666,
    "tx_count_1h": 2,
    "tx_count_24h": 10,
    "last_tx_at": "2026-10-19T10:15:00Z",
    "new_receiver": true,
    "new_device": false
  }
}
```

`model_features` chỉ cần có giá trị cho `TransferCompleted`. Các event khác có
thể bỏ field hoặc để `null`.

## Quy trình xử lý

### Event mới

1. Relay nhận event từ persistent subscription của EventStoreDB.
2. Relay decode transfer và xác định source account, receiver, device.
3. Đọc `model_feature_states` và kiểm tra receiver/device history.
4. Tính sáu feature từ state hiện tại.
5. Tạo envelope có `model_features`.
6. Publish envelope vào `anomaly.bank.events.v1`.
7. Sau khi Kafka xác nhận, cập nhật rolling state và receiver/device history.
8. ACK event trong EventStoreDB.

### Event không phải transfer

Các event khác vẫn được publish vào cùng topic với payload gốc. Relay không cần
tính model feature cho những event này.

## At-least-once và idempotency

Kafka relay hiện dùng at-least-once delivery. Có thể xảy ra trường hợp Kafka đã
nhận message nhưng relay crash trước khi ACK EventStoreDB. Vì vậy cần lưu thêm
decision theo `event_id`, ví dụ collection:

```text
model_feature_decisions
```

Document decision lưu:

```json
{
  "_id": "event-123",
  "features": {
    "avg_amount_7d": 6666666,
    "tx_count_1h": 2,
    "tx_count_24h": 10,
    "last_tx_at": "2026-10-19T10:15:00Z",
    "new_receiver": true,
    "new_device": false
  },
  "state_applied": false
}
```

Khi retry cùng `event_id`:

1. Dùng lại decision đã lưu, không tính feature lần nữa.
2. Publish lại đúng message feature trước đó.
3. Chỉ cập nhật state nếu `state_applied` chưa được đánh dấu.

State update phải idempotent theo `event_id` hoặc `state_version`, tránh cộng
amount và counter hai lần.

## Cache và hiệu năng

MongoDB là nguồn state bền vững. Sau này có thể thêm memory cache trong relay:

```text
Memory cache → MongoDB khi cache miss/checkpoint
MongoDB      → nguồn khôi phục sau khi relay restart
```

Không nên chỉ lưu summary trong memory vì sẽ mất state khi relay restart.

Mỗi giao dịch mục tiêu là:

```text
1 point lookup state
1 point lookup decision (idempotency)
1-2 point lookup receiver/device
1 publish Kafka
1 state update
```

Không còn query và aggregate toàn bộ transaction history cho từng giao dịch.

## Các thay đổi đã triển khai

1. Mở rộng `kafka.EventEnvelope` với `ModelFeatures`.
2. Thêm MongoDB connection/config cho `cmd/relay`.
3. Tạo feature state repository và các collection/index cần thiết.
4. Tạo feature calculator dùng rolling buckets.
5. Tạo idempotency repository theo `event_id`.
6. Enrich event trong relay trước khi gọi `producer.Publish`.
7. Chỉ ACK sau khi publish và state update hoàn tất.
8. Sửa `consumer-mock` để đọc/log `model_features`, không tự query hoặc tính
   lại lịch sử.
9. Bổ sung unit test cho rolling window, receiver/device mới và retry.

## Bootstrap và recovery state

Feature state phải được xây dựng theo đúng thứ tự event trước khi dùng cho dữ
liệu lịch sử. Relay lưu tiến độ đã xử lý trong collection
`model_feature_progress`. Khi persistent-subscription checkpoint vượt quá tiến
độ MongoDB, relay tự reset và replay state-only đến đúng checkpoint; các event
được replay không publish lại Kafka.

Có thể chạy recovery thủ công khi cần:

```bash
./relay --bootstrap-model-features
```

Phải dừng relay chính trước khi chạy lệnh này. Lệnh sẽ xóa và dựng lại state,
decision, receiver/device history từ banking stream mà không ghi message mới
vào Kafka.

`model_feature_decisions` có cleanup định kỳ cho các decision đã applied quá 90
ngày. Không xóa sớm hơn thời gian mà hệ thống còn cần replay EventStoreDB;
nếu cần replay xa hơn phải dùng state-only bootstrap.

Thiết kế này vẫn giữ nguyên một Kafka topic, giữ nguyên event payload gốc và
đưa phần tính feature về đúng phía producer như yêu cầu.
