import json

import pytest
from conftest import request_event

from app.contracts import ContractError, Prediction, PredictorResult
from app.worker import handle_message


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


class FakePredictor:
    def __init__(self, result=None, error=None):
        self.result = result or PredictorResult(prediction(), request={"eventId": "raw"})
        self.error = error
        self.event_ids = []

    def predict(self, event_id):
        self.event_ids.append(event_id)
        if self.error:
            raise self.error
        return self.result


class FakeOutput:
    def __init__(self, error=None):
        self.items = []
        self.error = error

    def publish(self, risk_event):
        if self.error:
            raise self.error
        self.items.append(risk_event)


class FakeTraining:
    def __init__(self, error=None):
        self.items = []
        self.error = error

    def record(self, event, request, prediction):
        if self.error:
            raise self.error
        self.items.append((event, request, prediction))


def test_handle_message_publishes_then_records():
    predictor, output, training = FakePredictor(), FakeOutput(), FakeTraining()
    result = handle_message(json.dumps(request_event()).encode(), predictor, output, training)

    assert result["transaction_id"] == "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
    assert result["event_id"] == "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70"
    assert predictor.event_ids == ["6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70"]
    assert len(output.items) == 1
    assert len(training.items) == 1
    assert training.items[0][1] == {"eventId": "raw"}


def test_non_request_event_is_committed_without_prediction():
    predictor, output, training = FakePredictor(), FakeOutput(), FakeTraining()
    result = handle_message(json.dumps(request_event(event_type="TransferCompleted")).encode(),
                            predictor, output, training)
    assert result is None
    assert predictor.event_ids == []
    assert output.items == []
    assert training.items == []


def test_trigger_timestamp_is_used_for_output():
    request = {
        "eventId": "raw",
        "transactionId": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
        "occurredAt": "2026-10-19T10:30:00Z",
    }
    predictor = FakePredictor(PredictorResult(prediction(), request=request))
    output, training = FakeOutput(), FakeTraining()
    result = handle_message(json.dumps(request_event()).encode(), predictor, output, training)
    assert result["occurred_at"] == "2026-10-19T10:30:00.123456789Z"
    assert training.items[0][0].occurred_at == "2026-10-19T10:30:00.123456789Z"


def test_transaction_mismatch_fails_before_publish():
    predictor = FakePredictor(PredictorResult(prediction(transaction_id="OTHER"),
                                              request={"eventId": "raw"}))
    output, training = FakeOutput(), FakeTraining()
    with pytest.raises(ContractError, match="transaction_id"):
        handle_message(json.dumps(request_event()).encode(), predictor, output, training)
    assert output.items == []
    assert training.items == []


def test_output_or_model_failure_prevents_training_record():
    predictor = FakePredictor()
    output = FakeOutput(error=RuntimeError("kafka"))
    training = FakeTraining()
    with pytest.raises(RuntimeError, match="kafka"):
        handle_message(json.dumps(request_event()).encode(), predictor, output, training)
    assert training.items == []

    predictor, output, training = (FakePredictor(error=RuntimeError("model")),
                                   FakeOutput(), FakeTraining())
    with pytest.raises(RuntimeError, match="model"):
        handle_message(json.dumps(request_event()).encode(), predictor, output, training)
    assert output.items == []
