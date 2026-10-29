FROM python:3.11-slim AS base

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg libgl1 libglib2.0-0 libportaudio2 && rm -rf /var/lib/apt/lists/*

COPY pyproject.toml README.md ./
COPY app ./app

RUN python -m pip install --no-cache-dir --upgrade pip && \
    python -m pip install --no-cache-dir '.[vision]'

FROM base AS checks
COPY tests ./tests
COPY config ./config
RUN python -m pip install --no-cache-dir '.[dev]'
RUN ruff check . && ruff format --check .
# RustFS-backed tests need a Docker daemon; CI runs that suite separately.
RUN pytest -q tests/test_real_vision.py tests/test_cors.py

FROM base AS runtime
RUN addgroup --system app && adduser --system --ingroup app app && chown -R app:app /app

EXPOSE 8090

USER app

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090"]
