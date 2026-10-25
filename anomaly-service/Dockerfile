FROM python:3.12-slim

WORKDIR /app
COPY anomaly/anomaly-service/pyproject.toml anomaly/anomaly-service/README.md ./
COPY anomaly/anomaly-service/app ./app
COPY anomaly/anomaly-service/models ./models
COPY GATv2/src /opt/GATv2/src

# Use the CPU wheel index so a CPU Swarm node does not pull the multi-GB CUDA
# distribution from PyPI.
RUN --network=host pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu "torch>=2.0"
RUN --network=host pip install --no-cache-dir .

ENV MODEL_BUNDLE_PATH=/app/models/gatv2-1m-baseline-v1 \
    GATV2_SRC_PATH=/opt/GATv2/src \
    METRICS_PORT=9100

EXPOSE 9100
CMD ["anomaly-ml-pipeline"]
