# Anomaly Python Service

Python service for stateless face verification processing using a clear front image of a Vietnamese CCCD and a live video with active liveness.

## Purpose

This service implements the CV/ML side of `specs/Face-Verification-Design-Plan.md`.

Its responsibility is limited to:

- receiving a CCCD front image and live video from `server/`
- extracting the portrait from the CCCD image
- evaluating active liveness from the video
- matching the live face against the CCCD portrait
- returning machine-readable scores, codes, and fallback messages

It does not own session policy, retry counting, duplicate checks, or persistence. Those business rules belong in `server/`.

## Scope

In scope:

- CCCD front image validation
- card detection and alignment
- portrait extraction
- live video preprocessing
- active liveness checks
- face embedding and 1:1 verification
- decision scoring and fallback reason messages

Out of scope:

- verification session storage
- duplicate/existence checks across users
- Mongo persistence
- full CCCD OCR
- passive-only liveness flows

## Architecture

```mermaid
flowchart LR
    Client[Client App] --> Gateway[Server / Go API]
    Gateway --> Py[Python Service]
    Py --> Gateway
```

Responsibilities by component:

- `client/`: captures CCCD front image and live video
- `server/`: owns auth, rate limiting, retry policy, duplicate checks, persistence, and orchestration
- `python-service/`: runs image/video processing and returns analysis results only

## Processing Pipeline

### 1. CCCD processing
1. Validate uploaded image format and file constraints.
   - Libraries:
     - `FastAPI` + `python-multipart`: nhận file upload multipart từ `server/`.
     - `app.files.read_limited_bytes`: chặn ảnh vượt kích thước cấu hình trước khi đi vào pipeline.
2. Detect the CCCD in the image.
   - Planned library:
     - `OpenCV`: phát hiện viền/thân thẻ CCCD trong ảnh đầu vào.
3. Align the card to a normalized perspective.
   - Planned library:
     - `OpenCV`: sửa phối cảnh, xoay và chuẩn hóa khung thẻ về cùng layout.
4. Crop the portrait region using a template-based layout.
   - Planned library:
     - `OpenCV`: crop vùng chân dung theo template sau khi thẻ đã được align.
5. Confirm a face exists in the cropped portrait.
   - Planned libraries:
     - `MediaPipe`: xác nhận có khuôn mặt trong vùng chân dung đã crop.
6. Run quality checks:
   - blur
   - brightness
   - glare
   - occlusion
   - face size
   - Planned libraries:
     - `OpenCV`: blur, brightness, glare và các chỉ số chất lượng ảnh cơ bản.
     - `MediaPipe`: ước lượng kích thước mặt, landmark và dấu hiệu che khuất.

### 2. Live video processing
1. Validate video format, duration, and size constraints.
   - Libraries:
     - `FastAPI` + `python-multipart`: nhận video upload multipart.
     - `app.files.read_limited_bytes`: chặn video vượt giới hạn dung lượng.
2. Extract candidate frames.
   - Planned library:
     - `OpenCV`: đọc video và trích các frame đại diện để đánh giá.
3. Detect the face across frames.
   - Planned libraries:
     - `MediaPipe`: phát hiện khuôn mặt và landmark theo từng frame.
4. Ensure a single consistent subject is present.
   - Planned libraries:
     - `MediaPipe`: kiểm tra số lượng khuôn mặt theo frame.
     - face embedding model: so khớp embedding giữa các frame để xác nhận cùng một người.
5. Evaluate active liveness challenges:
   - blink
   - turn left
   - turn right
   - Planned libraries:
     - `MediaPipe`: dùng eye landmarks cho blink và head pose/face landmarks cho quay trái, quay phải.
6. Select the best valid live frames for matching.
   - Planned libraries:
     - `OpenCV`: chọn frame đủ sáng, ít mờ, ít nhiễu.
     - `MediaPipe`: giữ lại frame có mặt rõ và landmark ổn định.

### 3. Face verification
1. Generate an embedding from the CCCD portrait.
   - Planned libraries:
     - face embedding model (`face_recognition` hoặc model license-safe tương đương): sinh vector đặc trưng từ ảnh CCCD.
2. Generate embeddings from the best live frames.
   - Planned libraries:
     - face embedding model (`face_recognition` hoặc model license-safe tương đương): sinh vector đặc trưng từ frame video hợp lệ.
3. Compute similarity scores.
   - Planned libraries:
     - face embedding model: tính khoảng cách/similarity giữa embedding ảnh CCCD và ảnh live.
4. Combine:
   - match score
   - liveness score
   - quality checks
   - Libraries:
     - `Pydantic`: chuẩn hóa response schema, score fields, reason codes và fallback messages.
     - `pydantic-settings`: nạp threshold cấu hình để ra quyết định nhất quán theo môi trường.
5. Return an analysis result to `server/`.
   - Libraries:
     - `FastAPI`: expose endpoint stateless cho `server/`.
     - `Pydantic`: serialize response JSON machine-readable.

## Libraries By Responsibility

### Current runtime and service libraries

- `FastAPI`: API layer cho các endpoint `/health`, `/extract-id-face`, `/run-liveness`, `/match-face`, `/verify-face`.
- `python-multipart`: parse file upload multipart cho ảnh CCCD và live video.
- `Pydantic`: định nghĩa request/response schema, decision, reason code và quality checks.
- `pydantic-settings`: quản lý cấu hình môi trường như file limits, thresholds và backend selection.
- `uvicorn`: ASGI server để chạy service cục bộ hoặc trong container.
- `httpx`: HTTP client support cho integration/test flows khi cần gọi service bất đồng bộ.
- `boto3`: phục vụ test fixtures upload/download media qua RustFS (S3-compatible).
- `pytest`: test framework cho API contract và integration tests.
- `ruff`: lint và format checks.

### Planned CV/ML libraries

- `OpenCV`: card detection, alignment, and basic image quality checks
- `MediaPipe`: landmarks, blink detection, head turn checks, and active liveness signals
- `face_recognition` or another license-safe embedding model: face embedding and comparison

Notes:

- Hiện tại pipeline trong `app/services/pipeline.py` vẫn là stub để giữ ổn định HTTP contract.
- Các thư viện CV/ML ở trên là mapping mục tiêu cho từng tính năng khi tích hợp backend xử lý thật.

## API Role

This service sits behind `server/`. It exposes stateless processing endpoints. `server/` is responsible for deciding whether a result becomes retryable, final, persisted, or reused.

Current implemented endpoints:

- `GET /health`
- `POST /v1/kyc/extract-id-face`
- `POST /v1/kyc/run-liveness`
- `POST /v1/kyc/match-face`
- `POST /v1/kyc/verify-face`

Response design notes:

- analysis responses include both `reason_code` and fallback `reason_message`
- no endpoint stores sessions or attempts
- the contract is designed so `server/` can add business rules without changing the CV boundary

## Decision Model

The combined verification endpoint returns one of:

- `VERIFIED`
- `RETRY_ALLOWED`

Common reason codes include:

- `CCCD_IMAGE_QUALITY_LOW`
- `CCCD_PORTRAIT_NOT_FOUND`
- `LIVE_FACE_NOT_FOUND`
- `MULTIPLE_FACES_DETECTED`
- `LIVE_VIDEO_QUALITY_LOW`
- `FACE_OCCLUDED`
- `LIVENESS_CHALLENGE_FAILED`
- `LIVENESS_NOT_CONFIDENT`
- `FACE_MISMATCH`
- `INTERNAL_ERROR`

## Security and Privacy

- treat CCCD portraits, live video, and embeddings as sensitive biometric data
- do not log raw image or video payloads in application logs
- keep thresholds and anti-spoof logic server-side
- assume all client traffic arrives through `server/`, not directly

## Implementation Notes

- Start with clear CCCD images only. Reject poor inputs early instead of trying to recover low-quality captures.
- Prefer template-based portrait extraction for MVP after alignment.
- Use active liveness as the primary anti-spoof layer for MVP.
- Keep thresholds configurable so they can be calibrated later with real test data.
- Keep the pipeline behind interfaces so OpenCV, MediaPipe, and embedding models can be swapped without changing route handlers.

## Reference

- internal specs: `specs/Face-Verification-Design-Plan.md`

## Local Development

### Start Locally

From `python-service/`:

```bash
python -m venv .venv
. .venv/bin/activate
pip install -e .[dev]
cp .env.example .env
uvicorn app.main:app --reload --host 0.0.0.0 --port 8090
```

This runs in the foreground, so logs are printed directly in the terminal.

If you want to keep a copy of the logs:

```bash
uvicorn app.main:app --reload --host 0.0.0.0 --port 8090 2>&1 | tee python-service.log
```

### Start With Nix

From `python-service/`:

```bash
nix develop
python -m venv .venv
. .venv/bin/activate
pip install -e .[dev]
cp .env.example .env
uvicorn app.main:app --reload --host 0.0.0.0 --port 8090
```

This also runs in the foreground, so logs are visible immediately.

### Start With Docker

From `python-service/`:

```bash
docker build -t anomaly-python-service .
docker run --rm -p 8090:8090 anomaly-python-service
```

Logs are printed in the current terminal because the container runs in the foreground.

If you want to run detached and inspect logs separately:

```bash
docker run -d --name anomaly-python-service -p 8090:8090 anomaly-python-service
docker logs -f anomaly-python-service
```

### Start With Docker Compose

From `server/`:

```bash
docker compose --profile app up --build anomaly-python-service rustfs
```

This keeps the compose stack attached, so logs stream in the current terminal.

If you want compose in the background and read logs separately:

```bash
docker compose --profile app up -d --build anomaly-python-service rustfs
docker compose logs -f anomaly-python-service
```

To also watch RustFS logs:

```bash
docker compose logs -f anomaly-python-service rustfs
```

### Nix

The shell includes:

- `python3.11`
- `pip`
- `virtualenv`
- `docker` client
- `ruff`

### Stub Mode For Local Testing

The service fails closed by default if no real vision backend is configured.

For local testing with the stub pipeline:

```bash
export APP_ENV=development
export VISION_PIPELINE_BACKEND=stub
export ALLOW_STUB_VISION_PIPELINE=true
uvicorn app.main:app --reload --host 0.0.0.0 --port 8090
```

### Lint And Format Checks

Use `ruff` locally to catch issues before CI:

```bash
ruff check .
ruff format --check .
```

These commands only report issues. They do not auto-format unless you run `ruff format .` yourself.

## Tests

Run the test suite from `python-service/`:

```bash
pytest
```

Test setup notes:

- tests use `boto3` against a temporary local `RustFS` Docker container
- fixtures upload generated test media objects into a bucket
- tests download those objects and submit them to the FastAPI endpoints
- teardown removes uploaded objects, deletes the bucket, and stops RustFS

Requirements:

- Docker daemon available on the host
- optional override for image: `RUSTFS_DOCKER_IMAGE`

This keeps test data lifecycle explicit without introducing persistence responsibilities into the service itself.

## Swagger

FastAPI Swagger UI is available at:

- `/docs`
- `/redoc`
- `/openapi.json`

## Current Implementation Status

- FastAPI application scaffolded
- Stateless analysis endpoints implemented
- Configurable thresholds and file limits via environment variables
- Vision pipeline abstracted behind an interface for later OpenCV, MediaPipe, and embedding integration
- Pytest suite added with RustFS-backed test fixtures
- Nix flake added for local development and test tooling
- Ruff lint and format checks configured through `pyproject.toml`

The current pipeline implementation is intentionally a stub so the HTTP contract and service boundary stay stable while the actual CV/ML layers are integrated.
