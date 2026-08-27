FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

COPY pyproject.toml README.md ./
COPY app ./app

RUN python -m pip install --no-cache-dir --upgrade pip && \
    python -m pip install --no-cache-dir .

RUN addgroup --system app && adduser --system --ingroup app app && chown -R app:app /app

EXPOSE 8090

USER app

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090"]
