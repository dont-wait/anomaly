# syntax=docker/dockerfile:1
FROM python:3.12-slim AS checks
WORKDIR /app
RUN pip install --no-cache-dir 'pytest>=8.4,<9.0' 'prometheus-client>=0.20,<1.0'
COPY pyproject.toml README.md ./
COPY app ./app
COPY tests ./tests
COPY models ./models
RUN python -m pytest -q

FROM python:3.12-slim AS runtime
WORKDIR /app
COPY pyproject.toml README.md ./
COPY app ./app
COPY models ./models
# Supply a clean, pinned GATv2 checkout with --build-context gatv2=<path>.
COPY --from=gatv2 src /opt/GATv2/src
RUN pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu 'torch>=2.0'
RUN pip install --no-cache-dir .
ENV MODEL_BUNDLE_PATH=/app/models/gatv2-1m-baseline-v1 \
    GATV2_SRC_PATH=/opt/GATv2/src \
    METRICS_PORT=9100
EXPOSE 9100
CMD ["anomaly-ml-pipeline"]
