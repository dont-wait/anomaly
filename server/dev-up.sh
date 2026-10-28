#!/usr/bin/env sh
set -eu

if [ -z "${KAFKA_ADVERTISED_HOST:-}" ] && [ -f .env ]; then
  KAFKA_ADVERTISED_HOST="$(sed -n 's/^KAFKA_ADVERTISED_HOST=\(.*\)$/\1/p' .env | tail -n 1 | sed 's/^"//; s/"$//')"
  export KAFKA_ADVERTISED_HOST
fi

if [ -z "${KAFKA_ADVERTISED_HOST:-}" ]; then
  if command -v hostname >/dev/null 2>&1; then
    KAFKA_ADVERTISED_HOST="$(hostname -I 2>/dev/null | awk '{print $1}')"
  fi
  if [ -z "${KAFKA_ADVERTISED_HOST:-}" ] && command -v ipconfig >/dev/null 2>&1; then
    KAFKA_ADVERTISED_HOST="$(ipconfig getifaddr en0 2>/dev/null || true)"
  fi
  export KAFKA_ADVERTISED_HOST="${KAFKA_ADVERTISED_HOST:-localhost}"
fi

echo "Kafka advertised host: ${KAFKA_ADVERTISED_HOST}"
exec docker compose --profile app up --build "$@"
