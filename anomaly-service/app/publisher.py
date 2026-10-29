"""Kafka output publisher; the producer is injected so tests stay offline."""
import hashlib
import hmac
import json
import math
import re
from datetime import datetime
from decimal import Decimal

from .contracts import PREDICTION_FIELDS, Prediction

DECISION_FIELDS = (
    "transfer_id", "source_event_id", "decision", "is_anomalous",
    "risk_score", "model_version", "evaluated_at",
)


def canonical_risk_decision(risk_event):
    """Match riskDecisionUnsigned in the server and consumer-mock Go publisher."""
    if set(risk_event) - {"signature"} != set(DECISION_FIELDS):
        raise ValueError("risk decision fields must match the server contract")
    unsigned = {key: risk_event[key] for key in DECISION_FIELDS}
    for key in ("transfer_id", "source_event_id", "model_version"):
        if not isinstance(unsigned[key], str) or not unsigned[key].strip():
            raise ValueError(f"{key} must be a nonempty string")
    decision = unsigned["decision"]
    if decision not in {"ALLOW", "BLOCK", "PENDING"}:
        raise ValueError("decision must be ALLOW, BLOCK or PENDING")
    anomalous = unsigned["is_anomalous"]
    if type(anomalous) is not bool or anomalous != (decision == "BLOCK"):
        raise ValueError("is_anomalous must agree with decision")
    score = unsigned["risk_score"]
    if (isinstance(score, bool) or not isinstance(score, (int, float))
            or not math.isfinite(score) or not 0 <= score <= 1):
        raise ValueError("risk_score must be finite and in [0, 1]")
    unsigned["risk_score"] = float(score)
    timestamp = unsigned["evaluated_at"]
    if not isinstance(timestamp, str) or not re.fullmatch(
        r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z", timestamp
    ):
        raise ValueError("evaluated_at must be an RFC3339 UTC timestamp")
    datetime.fromisoformat(timestamp.replace("Z", "+00:00"))
    if "." in timestamp:
        timestamp = timestamp[:-1].rstrip("0").rstrip(".") + "Z"
    unsigned["evaluated_at"] = timestamp
    canonical = "{" + ",".join(
        f'{json.dumps(key)}:{_go_json(unsigned[key])}' for key in DECISION_FIELDS
    ) + "}"
    return unsigned, canonical.encode("utf-8")


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
    """Use the field order and serialization of the server's ModelResponse struct."""
    if "transfer_id" in risk_event:
        return canonical_risk_decision(risk_event)
    unsigned = {key: value for key, value in risk_event.items() if key != "signature"}
    prediction = Prediction.from_payload(unsigned)
    unsigned = prediction.as_dict()
    canonical = "{" + ",".join(
        f'{json.dumps(key)}:{_go_json(unsigned[key])}' for key in PREDICTION_FIELDS
    ) + "}"
    return unsigned, canonical.encode("utf-8")


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
        if "transfer_id" in signed:
            key = signed["transfer_id"].encode()
        else:
            key = ":".join(signed[field] for field in (
                "transaction_id", "model_version", "feature_version"
            )).encode()
        value = json.dumps(signed, ensure_ascii=False, separators=(",", ":")).encode()
        self.producer.send(self.topic, key=key, value=value).get(timeout=self.timeout)
