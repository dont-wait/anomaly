import json

import pytest
from conftest import request_event_bytes

from app.contracts import Prediction
from app.events import select_trigger_event
from app.training import TrainingDataSink


class Future:
    def __init__(self):
        self.timeout = None

    def get(self, timeout):
        self.timeout = timeout
        return self


class FakeProducer:
    def __init__(self, error=None):
        self.calls = []
        self.error = error
        self.futures = []

    def send(self, topic, key=None, value=None):
        if self.error:
            raise self.error
        self.calls.append({"topic": topic, "key": key, "value": value})
        future = Future()
        self.futures.append(future)
        return future


def sample():
    event = select_trigger_event(request_event_bytes())
    prediction = Prediction.from_payload({
        "transaction_id": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
        "risk_score": 0.9821,
        "predicted_label": 1,
        "threshold": 0.9517,
        "model_version": "gatv2-device-control-v1",
        "feature_version": "schema-3-test",
    })
    request = {"eventId": event.event_id, "eventType": "TransferCreated", "payload": {"amount": 1}}
    return event, request, prediction


def test_record_writes_jsonl_and_publishes_kafka(tmp_path):
    producer = FakeProducer()
    path = tmp_path / "nested" / "predictions.jsonl"
    sink = TrainingDataSink(path, topic="anomaly.model.training.v1", fsync=False)
    event, request, prediction = sample()

    record = sink.record(event, request, prediction, producer=producer, timeout=7)

    line = json.loads(path.read_text().strip())
    assert line == record
    assert line["request"] == request
    assert line["response"]["risk_score"] == 0.9821
    assert line["event"]["event_id"] == event.event_id
    assert producer.calls[0]["topic"] == "anomaly.model.training.v1"
    assert producer.calls[0]["key"] == (
        b"aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1:gatv2-device-control-v1:schema-3-test"
    )
    assert json.loads(producer.calls[0]["value"]) == record
    assert producer.futures[0].timeout == 7


def test_duplicate_delivery_has_same_record_id(tmp_path):
    sink = TrainingDataSink(tmp_path / "predictions.jsonl", fsync=False)
    event, request, prediction = sample()
    first = sink.record(event, request, prediction)
    second = sink.record(event, request, prediction)
    assert first["record_id"] == second["record_id"]


def test_training_topic_requires_producer(tmp_path):
    sink = TrainingDataSink(tmp_path / "predictions.jsonl", topic="training", fsync=False)
    event, request, prediction = sample()
    with pytest.raises(ValueError, match="producer"):
        sink.record(event, request, prediction)


def test_injected_training_producer_is_used(tmp_path):
    producer = FakeProducer()
    sink = TrainingDataSink(tmp_path / "predictions.jsonl", topic="training", fsync=False,
                            producer=producer, timeout=9)
    event, request, prediction = sample()
    sink.record(event, request, prediction)
    assert producer.calls[0]["topic"] == "training"
    assert producer.futures[0].timeout == 9


def test_kafka_failure_happens_after_local_append(tmp_path):
    sink = TrainingDataSink(tmp_path / "predictions.jsonl", topic="training", fsync=False)
    event, request, prediction = sample()
    with pytest.raises(RuntimeError, match="kafka"):
        sink.record(event, request, prediction, producer=FakeProducer(error=RuntimeError("kafka")))
    assert tmp_path.joinpath("predictions.jsonl").exists()
