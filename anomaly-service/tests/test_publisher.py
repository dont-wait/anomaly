import hashlib
import hmac
import json
from pathlib import Path

import pytest

from app.contracts import Prediction
from app.publisher import KafkaOutputPublisher, canonical_risk_event, sign_risk_event


def assessment():
    prediction = Prediction("aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1", 0.9, 1, 0.5, "v1", "f1")
    return prediction.as_dict()


def test_signature_matches_fixed_canonical_bytes():
    event = assessment()
    canonical = (
        '{"transaction_id":"aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",'
        '"risk_score":0.9,"predicted_label":1,"threshold":0.5,'
        '"model_version":"v1","feature_version":"f1"}'
    )
    expected = hmac.new(b"test-secret", canonical.encode("utf-8"), hashlib.sha256).hexdigest()
    signed = sign_risk_event(event, "test-secret")
    assert signed == {**event, "signature": expected}
    assert "signature" not in event
    assert sign_risk_event(dict(reversed(list(event.items()))), "test-secret") == signed
    assert sign_risk_event({**event, "signature": "stale"}, "test-secret") == signed


def test_changed_payload_or_key_fails_signature_comparison():
    event = assessment()
    signature = sign_risk_event(event, "secret")["signature"]
    for changed in (
        {**event, "risk_score": 0.8},
        {**event, "transaction_id": "tx-2"},
        {**event, "feature_version": "f2"},
    ):
        assert not hmac.compare_digest(signature, sign_risk_event(changed, "secret")["signature"])
    assert not hmac.compare_digest(signature, sign_risk_event(event, "wrong-key")["signature"])


@pytest.mark.parametrize("key", [None, "", "   "])
def test_missing_key_cannot_publish(key):
    with pytest.raises(ValueError, match="KAFKA_RISK_DECISION_SIGNING_KEY"):
        KafkaOutputPublisher(None, "risk", signing_key=key)


def test_nonfinite_number_cannot_be_signed():
    with pytest.raises(ValueError):
        sign_risk_event({**assessment(), "risk_score": float("nan")}, "secret")


def test_publisher_sends_signed_payload_and_waits_for_ack():
    calls = []

    class Future:
        def get(self, *, timeout):
            calls.append(timeout)

    class Producer:
        def send(self, topic, *, key, value):
            calls.append((topic, key, json.loads(value)))
            return Future()

    event = assessment()
    publisher = KafkaOutputPublisher(Producer(), "risk", signing_key="secret", timeout=5)
    publisher.publish(event)
    assert calls == [
        ("risk", b"aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1:v1:f1", sign_risk_event(event, "secret")),
        5,
    ]


def test_publish_failure_propagates():
    class Producer:
        def send(self, *args, **kwargs):
            raise OSError("broker unavailable")

    publisher = KafkaOutputPublisher(Producer(), "risk", signing_key="secret")
    event = assessment()
    with pytest.raises(OSError, match="broker unavailable"):
        publisher.publish(event)


def test_signatures_match_shared_go_contract_vectors():
    vectors = json.loads((Path(__file__).parent / "fixtures/model_responses.json").read_text())
    for vector in vectors:
        signed = sign_risk_event(vector["message"], vector["key"])
        assert signed == vector["message"]
        assert canonical_risk_event(signed)[1].decode() == vector["canonical"]


def test_extra_unsigned_fields_are_rejected():
    with pytest.raises(ValueError, match="model response fields"):
        sign_risk_event({**assessment(), "decision": "ALLOW"}, "secret")
