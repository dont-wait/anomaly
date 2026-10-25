FROM python:3.12-slim
WORKDIR /app
COPY autoscaler.py .
RUN --network=host pip install --no-cache-dir docker==7.1.0
CMD ["python", "/app/autoscaler.py"]
