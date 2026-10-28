FROM python:3.12-slim

WORKDIR /app
COPY anomaly/anomaly-service/pyproject.toml anomaly/anomaly-service/README.md ./
COPY anomaly/anomaly-service/app ./app
COPY anomaly/anomaly-service/models ./models
COPY GATv2/src /opt/GATv2/src

RUN pip install --no-cache-dir .

ENV MODEL_BUNDLE_PATH=/app/models/gatv2-1m-baseline-v1 \
    GATV2_SRC_PATH=/opt/GATv2/src \
    METRICS_PORT=9100

EXPOSE 9100
CMD ["anomaly-ml-pipeline"]
