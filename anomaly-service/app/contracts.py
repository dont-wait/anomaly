"""Versioned output and training-record contracts."""
import hashlib
import json
import math
from dataclasses import dataclass
from datetime import datetime, timezone

PREDICTION_FIELDS = (
    "transaction_id",
    "risk_score",
    "predicted_label",
    "threshold",
    "model_version",
    "feature_version",
)
TRAINING_SCHEMA_VERSION = 1


class ContractError(ValueError):
    pass


@dataclass(frozen=True)
class Prediction:
    transaction_id: str
    risk_score: float
    predicted_label: int
    threshold: float
    model_version: str
    feature_version: str

    @classmethod
    def from_payload(cls, payload):
        if not isinstance(payload, dict):
            raise ContractError("model response must be an object")
        if set(payload) != set(PREDICTION_FIELDS):
            raise ContractError(f"model response fields must be exactly {PREDICTION_FIELDS}")
        for name in ("transaction_id", "model_version", "feature_version"):
            value = payload[name]
            if not isinstance(value, str) or not value.strip():
                raise ContractError(f"model response {name} must be a nonempty string")
        for name in ("risk_score", "threshold"):
            value = payload[name]
            if (isinstance(value, bool) or not isinstance(value, (int, float))
                    or not math.isfinite(value) or not 0 <= value <= 1):
                raise ContractError(f"model response {name} must be a finite number in [0, 1]")
        label = payload["predicted_label"]
        if isinstance(label, bool) or type(label) is not int or label not in (0, 1):
            raise ContractError("model response predicted_label must be integer 0 or 1")
        if label != int(payload["risk_score"] >= payload["threshold"]):
            raise ContractError(
                "model response predicted_label disagrees with risk_score/threshold"
            )
        return cls(
            transaction_id=payload["transaction_id"],
            risk_score=float(payload["risk_score"]),
            predicted_label=label,
            threshold=float(payload["threshold"]),
            model_version=payload["model_version"],
            feature_version=payload["feature_version"],
        )

    def as_dict(self):
        return {
            "transaction_id": self.transaction_id,
            "risk_score": self.risk_score,
            "predicted_label": self.predicted_label,
            "threshold": self.threshold,
            "model_version": self.model_version,
            "feature_version": self.feature_version,
        }

    @property
    def key(self):
        return f"{self.transaction_id}:{self.model_version}:{self.feature_version}"


@dataclass(frozen=True)
class PredictorResult:
    prediction: Prediction
    request: dict


def build_risk_event(event, prediction):
    prediction = _prediction(prediction)
    return build_risk_decision(
        event, prediction, decision="BLOCK" if prediction.predicted_label else "ALLOW"
    )


def build_risk_decision(event, prediction, *, decision, evaluated_at=None):
    """Adapt a prediction to the server contract using an explicitly supplied decision."""
    prediction = _prediction(prediction)
    if prediction.transaction_id != event.transaction_id:
        raise ContractError("model response transaction_id differs from the Kafka trigger")
    if decision not in {"ALLOW", "BLOCK", "PENDING"}:
        raise ContractError("decision must be ALLOW, BLOCK or PENDING")
    return {
        "transfer_id": prediction.transaction_id,
        "source_event_id": event.event_id,
        "decision": decision,
        "is_anomalous": decision == "BLOCK",
        "risk_score": prediction.risk_score,
        "model_version": prediction.model_version,
        "evaluated_at": evaluated_at or utc_now(),
    }


def build_training_record(event, request, prediction, *, recorded_at=None):
    prediction = _prediction(prediction)
    request = request.as_dict() if hasattr(request, "as_dict") else dict(request)
    event_payload = {
        "event_id": event.event_id,
        "event_type": event.event_type,
        "stream": event.stream,
        "stream_revision": event.stream_revision,
        "occurred_at": event.occurred_at,
        "schema_version": event.schema_version,
    }
    record = {
        "schema_version": TRAINING_SCHEMA_VERSION,
        "recorded_at": recorded_at or utc_now(),
        "event": event_payload,
        "request": request,
        "response": prediction.as_dict(),
        "prediction_key": prediction.key,
    }
    record["record_id"] = hashlib.sha256(_canonical(
        {"event": event_payload, "request": request, "response": prediction.as_dict()}
    ).encode()).hexdigest()
    return record


def _prediction(value):
    if isinstance(value, Prediction):
        return value
    return Prediction.from_payload(value)


def _canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def utc_now():
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")
