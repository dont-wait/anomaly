"""Kafka output publisher; the producer is injected so tests stay offline."""
import hashlib
import hmac
import json
import math
import re
from datetime import datetime
from decimal import Decimal

from .contracts import Prediction

RISK_EVENT_METADATA_FIELDS = ("schema_version", "event_id", "occurred_at", "scored_at")
TIMESTAMP_FIELDS = ("occurred_at", "scored_at")


def _go_timestamp(name, value):
    """Match how Go re-marshals a decoded UTC time.Time: RFC3339Nano without trailing zeros."""
    if not isinstance(value, str) or not re.fullmatch(
        r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z", value
    ):
        raise ValueError(f"{name} must be an RFC3339 UTC timestamp")
    datetime.fromisoformat(value[:19] + "+00:00")
    if "." in value:
        value = value[:-1].rstrip("0").rstrip(".") + "Z"
    return value


def _go_json(value):
    """Match encoding/json for the scalar types in riskassessment.ModelResponse."""
    if isinstance(value, float):
        if not math.isfinite(value):
            raise ValueError("risk score must be finite")
        text = repr(value)
        if value == 0 or 1e-6 <= abs(value) < 1e21:
            return format(Decimal(text), "f").removesuffix(".0")
        return re.sub(r"e([+-])0+(\d+)$", r"e\1\2", text)
    text = json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False)
    for char, escaped in (
        ("<", "\\u003c"), (">", "\\u003e"), ("&", "\\u0026"),
        ("\u2028", "\\u2028"), ("\u2029", "\\u2029"),
    ):
        text = text.replace(char, escaped)
    return text


def canonical_risk_event(risk_event):
    """Match riskassessment.CanonicalJSON in the server: a sorted-key Go map."""
    unsigned = {key: value for key, value in risk_event.items() if key != "signature"}
    metadata = {}
    for key in RISK_EVENT_METADATA_FIELDS:
        if key not in unsigned:
            raise ValueError(f"risk event {key} is required")
        metadata[key] = unsigned.pop(key)
    if type(metadata["schema_version"]) is not int or metadata["schema_version"] != 1:
        raise ValueError("schema_version must be 1")
    if not isinstance(metadata["event_id"], str) or not metadata["event_id"].strip():
        raise ValueError("event_id must be a nonempty string")
    for key in TIMESTAMP_FIELDS:
        metadata[key] = _go_timestamp(key, metadata[key])
    normalized = {**metadata, **Prediction.from_payload(unsigned).as_dict()}
    canonical = "{" + ",".join(
        f"{json.dumps(key)}:{_go_json(normalized[key])}" for key in sorted(normalized)
    ) + "}"
    return normalized, canonical.encode("utf-8")


def sign_risk_event(risk_event, signing_key):
    """Sign a server-compatible assessment or decision using HMAC-SHA256."""
    if not isinstance(signing_key, str) or not signing_key.strip():
        raise ValueError("KAFKA_RISK_DECISION_SIGNING_KEY is required")
    unsigned, canonical = canonical_risk_event(risk_event)
    signature = hmac.new(signing_key.encode("utf-8"), canonical, hashlib.sha256).hexdigest()
    return {**unsigned, "signature": signature}


class KafkaOutputPublisher:
    def __init__(self, producer, topic, *, signing_key, timeout=30):
        if not topic:
            raise ValueError("output topic is required")
        if not isinstance(signing_key, str) or not signing_key.strip():
            raise ValueError("KAFKA_RISK_DECISION_SIGNING_KEY is required")
        self.producer = producer
        self.topic = topic
        self.timeout = timeout
        self._signing_key = signing_key

    def publish(self, risk_event):
        signed = sign_risk_event(risk_event, self._signing_key)
        key = ":".join(signed[field] for field in (
            "transaction_id", "model_version", "feature_version"
        )).encode()
        value = json.dumps(signed, ensure_ascii=False, separators=(",", ":")).encode()
        self.producer.send(self.topic, key=key, value=value).get(timeout=self.timeout)
