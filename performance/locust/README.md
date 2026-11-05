# Locust performance tests

Hai scenario được cung cấp:

- `RealtimeAlertUser`: tạo transfer và đo thời gian đến khi risk assessment xuất hiện qua endpoint tra cứu theo transaction ID.
- `RealtimeAlertSseUser`: scenario SSE tùy chọn, giữ connection `/api/admin/monitor/stream` và đo thời gian đến khi nhận alert.
- `AuditUser`: tải các API monitor, account timeline, event explorer và transaction history.

## Cài đặt

```bash
cd /path/to/anomaly
python3 -m venv performance/locust/.venv
. performance/locust/.venv/bin/activate
python -m pip install --upgrade pip
python -m pip install -r performance/locust/requirements.txt
cp performance/locust/.env.example performance/locust/.env
# sửa performance/locust/.env và điền credential thật
```

## Chạy web UI

```bash
python -m locust -f performance/locust/locustfile.py
```

Mở `http://localhost:8089`, sau đó chọn số user và spawn rate.

## Chạy headless

```bash
python -m locust -f performance/locust/locustfile.py \
  --headless \
  --users 20 \
  --spawn-rate 2 \
  --run-time 5m \
  --html /tmp/locust-report.html \
  --csv /tmp/locust-results
```

## Lưu ý

- Cần dùng account test riêng; mỗi user Locust tạo transaction mới với `Idempotency-Key` riêng.
- Luồng Locust gọi confirm với OTP rỗng khi `LOCUST_CONFIRM_WITHOUT_OTP=true`; API server phải chạy với `DISABLE_TRANSFER_OTP=true` trong môi trường test.
- `RealtimeAlertUser` đo latency đến lúc assessment xuất hiện ở `GET /api/admin/monitor`; đây là polling-based measurement.
- `RealtimeAlertSseUser` mặc định không chạy cùng các user khác; chạy riêng bằng cách đặt tên class ở cuối command: `python -m locust -f performance/locust/locustfile.py RealtimeAlertSseUser`.
- Trong lúc chạy cần quan sát thêm Kafka consumer lag, CPU/memory của API/anomaly-service và MongoDB latency.
