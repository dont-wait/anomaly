FROM python:3.12-slim AS checks
WORKDIR /app
COPY autoscaler.py .
RUN python -c "import ast; from pathlib import Path; ast.parse(Path('autoscaler.py').read_text())"

FROM python:3.12-slim AS runtime
WORKDIR /app
COPY autoscaler.py .
RUN pip install --no-cache-dir docker==7.1.0
CMD ["python", "/app/autoscaler.py"]
