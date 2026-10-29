import json
from pathlib import Path

import pytest
from conftest import request_event_bytes

from app.contracts import Prediction, build_risk_decision
from app.events import select_trigger_event
from app.publisher import KafkaOutputPublisher, canonical_risk_event, sign_risk_event


@pytest.mark.parametrize("decision", ["ALLOW", "BLOCK", "PENDING"])
def test_adapter_requires_explicit_decision_and_matches_mock_fields(decision):
    event = select_trigger_event(request_event_bytes())
    prediction = Prediction(event.transaction_id, 0.9, 1, 0.5, "model-v1", "f1")
    result = build_risk_decision(
        event, prediction, decision=decision, evaluated_at="2026-10-19T10:30:01.120Z"
    )
    assert result == {
        "transfer_id": event.transaction_id,
        "source_event_id": event.event_id,
        "decision": decision,
        "is_anomalous": decision == "BLOCK",
        "risk_score": 0.9,
        "model_version": "model-v1",
        "evaluated_at": "2026-10-19T10:30:01.120Z",
    }
    signed = sign_risk_event(result, "test-only-secret")
    assert signed["evaluated_at"] == "2026-10-19T10:30:01.12Z"
    assert "signature" not in result


def test_decision_signatures_match_shared_go_vectors():
    vectors = json.loads((Path(__file__).parent / "fixtures/risk_decisions.json").read_text())
    for vector in vectors:
        assert sign_risk_event(vector["message"], vector["key"]) == vector["message"]
        assert canonical_risk_event(vector["message"])[1].decode() == vector["canonical"]


def test_decision_publisher_uses_transfer_id_as_kafka_key():
    vector = json.loads((Path(__file__).parent / "fixtures/risk_decisions.json").read_text())[0]
    calls = []

    class Future:
        def get(self, *, timeout):
            calls.append(timeout)

    class Producer:
        def send(self, topic, *, key, value):
            calls.append((topic, key, json.loads(value)))
            return Future()

    publisher = KafkaOutputPublisher(Producer(), "risk", signing_key=vector["key"], timeout=5)
    publisher.publish(vector["message"])
    assert calls == [("risk", vector["message"]["transfer_id"].encode(), vector["message"]), 5]
