import math

import pytest
from conftest import request_event_bytes

from app.contracts import (
    ContractError,
    Prediction,
    PredictorResult,
    build_risk_event,
    build_training_record,
)
from app.events import select_trigger_event


def prediction(**overrides):
    payload = {
        "transaction_id": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
        "risk_score": 0.9821,
        "predicted_label": 1,
        "threshold": 0.9517,
        "model_version": "gatv2-device-control-v1",
        "feature_version": "schema-3-test",
    }
    payload.update(overrides)
    return Prediction.from_payload(payload)


def selected():
    return select_trigger_event(request_event_bytes())


def test_prediction_contract_rejects_drift():
    assert prediction().key == (
        "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1:gatv2-device-control-v1:schema-3-test"
    )
    for payload in (
        {"transaction_id": "TX001", "risk_score": 0.9, "predicted_label": 1,
         "threshold": 0.7, "model_version": "M", "feature_version": "F", "extra": True},
        {"transaction_id": "", "risk_score": 0.9, "predicted_label": 1,
         "threshold": 0.7, "model_version": "M", "feature_version": "F"},
        {"transaction_id": "TX001", "risk_score": 1.1, "predicted_label": 1,
         "threshold": 0.7, "model_version": "M", "feature_version": "F"},
        {"transaction_id": "TX001", "risk_score": 0.9, "predicted_label": 0,
         "threshold": 0.7, "model_version": "M", "feature_version": "F"},
        {"transaction_id": "TX001", "risk_score": math.nan, "predicted_label": 1,
         "threshold": 0.7, "model_version": "M", "feature_version": "F"},
    ):
        with pytest.raises(ContractError):
            Prediction.from_payload(payload)


def test_risk_event_contains_join_metadata_and_prediction():
    event = selected()
    risk = build_risk_event(event, prediction(), scored_at="2026-10-19T10:30:01Z")
    assert risk["schema_version"] == 1
    assert risk["event_id"] == event.event_id
    assert risk["transaction_id"] == event.transaction_id
    assert risk["occurred_at"] == "2026-10-19T10:30:00.123456789Z"
    assert risk["scored_at"] == "2026-10-19T10:30:01Z"
    assert risk["risk_score"] == 0.9821
    assert risk["predicted_label"] == 1
    assert risk["model_version"] == "gatv2-device-control-v1"


def test_training_record_is_stable_for_duplicate_delivery():
    event = selected()
    request = {"eventId": event.event_id, "eventType": "TransferCreated", "payload": {"amount": 1}}
    first = build_training_record(event, request, prediction(),
                                  recorded_at="2026-10-19T10:30:01Z")
    second = build_training_record(event, request, prediction(),
                                   recorded_at="2026-10-19T10:35:00Z")
    assert first["record_id"] == second["record_id"]
    assert first["request"] == request
    assert first["response"]["risk_score"] == 0.9821
    assert first["event"]["stream_revision"] == 123
    assert first["event"]["event_type"] == "TransferCreated"
    assert first["prediction_key"].startswith("aaaaaaa1-")
    assert first["recorded_at"] == "2026-10-19T10:30:01Z"


def test_predictor_result_carries_raw_request_for_training():
    event = selected()
    result = PredictorResult(prediction=prediction(), request={"eventId": event.event_id})
    assert result.request["eventId"] == event.event_id
