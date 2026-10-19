# Anomaly KYC service

Face verification 1:1 giữa chân dung trên ảnh căn cước và video camera. Luồng realtime là `mobile → Go → Python → Go → mobile`; Python nằm sau Go, không nhận JWT người dùng.

Backend `real` đã có adapter ONNX, MediaPipe Face Landmarker, tách chân dung theo ROI của layout thẻ, giải mã video bằng FFmpeg, kiểm tra chuỗi quay đầu/chớp mắt và tổng hợp embedding nhiều frame. **Repo không cung cấp weights hoặc ngưỡng đã hiệu chuẩn.** Phải cấp artifact/cấu hình đúng model trước khi chạy thực tế. Không có fallback từ real sang stub.

Spec chi tiết: [Face-Verification-Design-Plan](../specs/Face-Verification-Design-Plan.md).

## Chuẩn bị mô hình

Tạo thư mục riêng, ví dụ `kyc-service/models/`, chứa:

```text
models/
  manifest.json
  vision.toml
  face.onnx
  face_landmarker.task
```

1. Cung cấp model embedding ONNX có quyền sử dụng phù hợp và MediaPipe Face Landmarker task.
2. Điền [manifest.example.json](config/manifest.example.json): input/output names, shape, RGB/BGR, tensor layout, normalization và 5 landmark đích phải đúng preprocessing của model. Các số trong mẫu chỉ minh họa, không phải cấu hình dùng chung cho mọi model.
3. Tính SHA-256 của cả hai artifact, điền manifest. Model load một lần, kiểm tra input/output, warm-up và kiểm tra embedding hữu hạn/chuẩn hóa L2.
4. Sao chép [vision.example.toml](config/vision.example.toml) thành `vision.toml`, hiệu chuẩn matching/consistency/quality/liveness trên dữ liệu thực. Chỉ đặt `calibrated=true` khi đã có báo cáo tương ứng. Mẫu cố ý không chạy được mặc định.
5. Xác nhận `left_yaw_sign` trên video gốc chưa mirror. Xác nhận layout thẻ và `portrait_roi`; hiện hỗ trợ một ROI được cấu hình trên thẻ ngang đã căn phối cảnh. Chưa có bộ phân loại tự động nhiều thế hệ căn cước hoặc kiểm định thẻ thật/giả.

Adapter dùng cùng model embedding cho ảnh thẻ và frame video. Score là cosine similarity được ánh xạ sang [0,1], tổng hợp bằng median + tỉ lệ frame đạt; không phải xác suất danh tính. Mô hình/cấu hình không hợp lệ làm startup real thất bại.

## Chạy local

Dùng Python 3.11 và FFmpeg. Với Nix:

```bash
cd kyc-service
nix develop
python -m venv .venv
.venv/bin/python -m pip install -e '.[vision,dev]'
cp .env.example .env
```

Điền trong `.env`:

```dotenv
APP_ENV=development
VISION_PIPELINE_BACKEND=real
ALLOW_STUB_VISION_PIPELINE=false
KYC_SERVICE_TOKEN=<secret-ngau-nhien-it-nhat-32-ky-tu>
VISION_CONFIG_PATH=models/vision.toml
FACE_MODEL_MANIFEST_PATH=models/manifest.json
```

Đặt cùng `KYC_SERVICE_TOKEN` trong `server/.env`, giữ `KYC_SERVICE_URL=http://localhost:8090`. Khởi động Python bằng `make start` hoặc:

```bash
.venv/bin/python -m uvicorn app.main:app --host 0.0.0.0 --port 8090
```

`make start` đọc cấu hình từ `.env`, không ép dùng stub. Nix shell cung cấp FFmpeg và shared libraries cho wheels CV. Nếu thay đổi dependency, dùng môi trường ảo sạch; tránh cài đồng thời nhiều package OpenCV cùng cung cấp namespace `cv2`.

`GET /health` phản ánh tiến trình; `GET /ready` kiểm tra pipeline. Default `disabled` phục vụ health/docs nhưng từ chối phân tích. `stub` chỉ dành cho test hoặc override development có chủ ý; staging/production từ chối stub. Mobile/Go cũng từ chối kết quả verified không có evidence thực, vì vậy không dùng stub để mở tài khoản.

## Realtime và kết quả cuối

Go tạo attempt sau KYC auth, giữ ảnh tạm trong Redis theo TTL, cấp ticket một lần rồi kiểm tra Origin trước upgrade WebSocket. Mobile quay 12 giây, gửi binary WebM/VP8 khoảng mỗi 250 ms từ một recorder. MIME khác, gồm MP4 trên thiết bị chưa được kiểm chứng decode incremental, bị từ chối rõ ràng.

Python giữ context tạm theo socket, tách chân dung một lần, decode liên tục (15 FPS), chạy landmark/liveness và embedding khoảng 5 Hz; không dùng tên file hoặc timer UI để ra kết quả. Client chỉ cập nhật động tác theo progress server và giữ preview camera trực tiếp.

- Nén video không bị bỏ chunk khi mạng chậm. Client kiểm tra buffer/ACK, giới hạn dung lượng và hủy attempt khi vượt ngân sách.
- `progress` chỉ là bằng chứng tạm. `stream_finished` xác nhận đã kết thúc luồng, chưa xác minh tài khoản.
- Go chốt SHA-256 chuỗi chunk, xóa bytes ảnh khỏi record tạm và ràng buộc ảnh/video với request complete.
- Complete chạy lại pipeline thật trên đúng video hoàn chỉnh; Go kiểm tra quality checks, trạng thái, score, request ID và provenance trước khi lưu.
- Media verified vào object storage riêng tư. Mongo lưu điểm, evidence/version, metadata/hash; OCR vẫn `not_run`.
- Lỗi mạng khi complete cho gửi lại cùng video và idempotency key. Video chỉ giữ trong RAM của phiên UI, không ghi localStorage.

Trước khi upload, Go ghi kế hoạch reconciliation vào Redis. Worker chạy mỗi phút, kiểm tra các upload sau khoảng chờ 5 phút với hồ sơ canonical rồi chỉ xóa object chưa được session nào tham chiếu. Lỗi đọc hồ sơ hoặc xóa object giữ nguyên công việc để thử lại; kế hoạch này tồn tại qua API restart và xử lý cả upload/commit có kết quả không rõ do timeout.

Go mặc định giới hạn 3 lần không đạt/24 giờ theo tài khoản. Lỗi hệ thống không trừ lượt. Policy, idempotency và ticket lưu Redis dùng chung giữa replica. Các biến phía Go: `KYC_MAX_ATTEMPTS`, `KYC_ATTEMPT_WINDOW`, `KYC_ATTEMPT_TTL`, `KYC_MAX_CONCURRENT_ATTEMPTS`, `KYC_STARTS_PER_MINUTE`. Attempt đang hoạt động có lease, giới hạn số ảnh tạm toàn cục; Redis phải được bảo vệ như dữ liệu sinh trắc học. TTL là thời hạn đọc logic; retention của Redis AOF/backup cần quản lý ở hạ tầng.

Chạy migration Mongo `000009_real_kyc_evidence` trước phiên bản Go mới để cho phép `passed/failed`, score và evidence. Hồ sơ cũ giữ nguyên `not_run`. Rollback migration gỡ validator để không làm mất evidence đã ghi; cần xét lại validator trước khi đưa bản cũ vào vận hành.

## Docker Compose

Image cài `.[vision]` cùng FFmpeg/system libraries. Compose mount thư mục model read-only:

```dotenv
KYC_MODEL_DIR=/duong-dan-tuyet-doi/models
KYC_SERVICE_TOKEN=<cung-secret-voi-Go>
VISION_PIPELINE_BACKEND=real
```

Trong `server/`:

```bash
docker compose --profile app up --build anomaly-kyc-service
```

Model directory phải có `manifest.json`, `vision.toml` và các artifact tham chiếu. Không commit media hoặc weights. Reverse proxy của Go cần hỗ trợ WebSocket upgrade và timeout cho attempt; không bật buffering luồng này. Mở dịch vụ Python chỉ cho Go/mạng nội bộ.

## Kiểm chứng

```bash
ruff check .
ruff format --check .
pytest tests/test_real_vision.py tests/test_cors.py
```

`test_real_vision.py` kiểm tra fail-closed, checksum/config, thứ tự động tác, blink nhiều frame, coverage, tổng hợp điểm và decoder WebM thật phát frame trước EOF. Đây là kiểm tra kỹ thuật, không phải đánh giá nhận dạng người thật.

Test API cũ dùng stub và có fixture RustFS/Docker cho một số trường hợp. Chạy `pytest` toàn bộ khi Docker sẵn sàng. Phía Go có test Redis thực (đặt `KYC_TEST_REDIS_SERVER` hoặc có `redis-server` trên PATH) và có thể chạy `go test -race ./internal/infrastructure/kyc`.

Trước khi triển khai cần nghiệm thu model trên căn cước/video thật, đo FMR/FNMR, preview FPS, latency, thiết bị/codec, ánh sáng và mạng yếu theo spec. Chưa có dữ liệu để xác nhận các ngưỡng hoặc mục tiêu hiệu năng đạt.

## Giới hạn

Kiểm tra động tác là active challenge checking, chưa phải PAD chống ảnh màn hình, replay hoặc deepfake có đánh giá chuyên biệt. Challenge cố định có thể bị phát lại. Kết nối realtime không tự chứng minh video quay mới; cần nonce/challenge ngẫu nhiên và giải pháp chống injection nếu mở rộng yêu cầu đó.

Face match chứng minh độ tương đồng khuôn mặt trong media. Chưa có OCR đối chiếu thông tin, NFC/chip hoặc truy vấn cơ quan cấp; không được mô tả là xác thực pháp lý của căn cước. Không lưu embedding mặc định hoặc dùng dữ liệu người dùng để huấn luyện.
