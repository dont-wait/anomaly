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
2. Detect the CCCD in the image.
3. Align the card to a normalized perspective.
4. Crop the portrait region using a template-based layout.
5. Confirm a face exists in the cropped portrait.
6. Run quality checks:
   - blur
   - brightness
   - glare
   - occlusion
   - face size

### 2. Live video processing
1. Validate video format, duration, and size constraints.
2. Extract candidate frames.
3. Detect the face across frames.
4. Ensure a single consistent subject is present.
5. Evaluate active liveness challenges:
   - blink
   - turn left
   - turn right
6. Select the best valid live frames for matching.

### 3. Face verification
1. Generate an embedding from the CCCD portrait.
2. Generate embeddings from the best live frames.
3. Compute similarity scores.
4. Combine:
   - match score
   - liveness score
   - quality checks
5. Return an analysis result to `server/`.

## Suggested Tech Stack

- `OpenCV`: card detection, alignment, and basic image quality checks
- `MediaPipe`: landmarks, blink detection, head turn checks, and active liveness signals
- `face_recognition` or another license-safe embedding model: face embedding and comparison
- `FastAPI`: service API layer

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

1. Create a virtual environment.
2. Install dependencies:

```bash
pip install -e .
```

3. Copy `.env.example` to `.env` and adjust settings if needed.
4. Run the service:

```bash
uvicorn app.main:app --reload --host 0.0.0.0 --port 8090
```

### Nix

From `python-service/`:

```bash
nix develop
```

The shell includes:

- `python3.11`
- `pip`
- `virtualenv`
- `docker` client
- `ruff`

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
