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
        self.result = result or PredictorResult(
            prediction(), request={"eventId": "raw", "occurredAt": "2026-10-19T10:30:00Z"}
        )
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

    assert result["transfer_id"] == "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
    assert result["source_event_id"] == "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70"
    assert result["decision"] == "BLOCK"
    assert result["is_anomalous"] is True
    assert predictor.event_ids == ["6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70"]
    assert len(output.items) == 1
    assert len(training.items) == 1
    assert training.items[0][1] == {"eventId": "raw", "occurredAt": "2026-10-19T10:30:00Z"}


@pytest.mark.parametrize("score,label,decision", [(0.08, 0, "ALLOW"), (0.9821, 1, "BLOCK")])
def test_worker_publishes_signed_decision_using_model_result(score, label, decision):
    from app.publisher import KafkaOutputPublisher, sign_risk_event

    sent = []

    class Future:
        def get(self, *, timeout):
            assert timeout == 30

    class Producer:
        def send(self, topic, *, key, value):
            sent.append((key, json.loads(value)))
            return Future()

    model_result = prediction(risk_score=score, predicted_label=label)
    predictor = FakePredictor(PredictorResult(
        model_result, request={"occurredAt": "2026-10-19T10:30:00Z"}
    ))
    training = FakeTraining()
    publisher = KafkaOutputPublisher(Producer(), "risk", signing_key="test-only-secret")
    result = handle_message(json.dumps(request_event()).encode(), predictor, publisher, training)
    assert result["decision"] == decision
    assert result["is_anomalous"] is bool(label)
    assert sent == [
        (model_result.transaction_id.encode(), sign_risk_event(result, "test-only-secret"))
    ]
    assert training.items[0][2].as_dict() == model_result.as_dict()


def test_non_request_event_is_committed_without_prediction():
    predictor, output, training = FakePredictor(), FakeOutput(), FakeTraining()
    result = handle_message(
        json.dumps(request_event(event_type="TransferCompleted")).encode(),
        predictor,
        output,
        training,
    )
    assert result is None
    assert predictor.event_ids == []
    assert output.items == []
    assert training.items == []


def test_canonical_request_timestamp_is_used_for_training():
    request = {
        "eventId": "raw",
        "transactionId": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
        "occurredAt": "2026-10-19T10:30:00Z",
    }
    predictor = FakePredictor(PredictorResult(prediction(), request=request))
    output, training = FakeOutput(), FakeTraining()
    result = handle_message(json.dumps(request_event()).encode(), predictor, output, training)
    assert "occurred_at" not in result
    assert training.items[0][0].occurred_at == request["occurredAt"]
    assert training.items[0][0].occurred_at_us == 1792405800000000


def test_transaction_mismatch_fails_before_publish():
    predictor = FakePredictor(
        PredictorResult(
            prediction(transaction_id="OTHER"),
            request={"eventId": "raw", "occurredAt": "2026-10-19T10:30:00Z"},
        )
    )
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

    predictor, output, training = (
        FakePredictor(error=RuntimeError("model")),
        FakeOutput(),
        FakeTraining(),
    )
    with pytest.raises(RuntimeError, match="model"):
        handle_message(json.dumps(request_event()).encode(), predictor, output, training)
    assert output.items == []


def test_consumer_loop_commits_on_ignored_and_poison_messages():
    from app.worker import run

    class FakeMessage:
        def __init__(self, value, offset=0, partition=0):
            self.value = value
            self.offset = offset
            self.partition = partition

    class FakeConsumer:
        def __init__(self, messages):
            self.messages = messages
            self.commit_count = 0
            self.closed = False

        def __iter__(self):
            return iter(self.messages)

        def commit(self):
            self.commit_count += 1

        def close(self):
            self.closed = True

    messages = [
        FakeMessage(
            json.dumps(request_event(event_type="TransferCompleted")).encode(), offset=0
        ),  # ignored
        FakeMessage(b"not-valid-json{{{", offset=1),  # poison pill
        FakeMessage(json.dumps(request_event()).encode(), offset=2),  # valid
    ]
    consumer = FakeConsumer(messages)
    predictor = FakePredictor()
    output = FakeOutput()
    training = FakeTraining()

    class Config:
        kafka_brokers = ["localhost:9092"]
        input_topic = "test"
        output_topic = "test-out"
        training_topic = "test-train"
        model_bundle_path = "/models"

    run(Config(), consumer=consumer, predictor=predictor, output=output, training=training)

    assert consumer.commit_count == 3
    assert consumer.closed is True
    assert len(output.items) == 1


@pytest.mark.parametrize("stage", ["model", "output", "training", "identity"])
def test_processing_failure_is_not_committed_or_followed_by_later_message(stage):
    from types import SimpleNamespace

    from app.predictor import UnsupportedIdentityError
    from app.worker import run

    class Consumer:
        def __init__(self):
            self.commits = 0
            self.polled = 0
            self.closed = False

        def __iter__(self):
            for offset in range(2):
                self.polled += 1
                yield SimpleNamespace(value=json.dumps(request_event()).encode(), offset=offset)

        def commit(self):
            self.commits += 1

        def close(self):
            self.closed = True

    config = SimpleNamespace(
        kafka_brokers=["localhost:9092"],
        input_topic="in",
        output_topic="out",
        training_topic="train",
        model_bundle_path="bundle",
    )
    error = (
        UnsupportedIdentityError("unsupported account") if stage == "identity" else OSError(stage)
    )
    predictor = FakePredictor(error=error if stage in {"model", "identity"} else None)
    output = FakeOutput(error=error if stage == "output" else None)
    training = FakeTraining(error=error if stage == "training" else None)
    consumer = Consumer()

    with pytest.raises(type(error)):
        run(config, consumer=consumer, predictor=predictor, output=output, training=training)

    assert consumer.commits == 0
    assert consumer.polled == 1
    assert consumer.closed
    assert len(output.items) == (1 if stage == "training" else 0)
    assert training.items == []


@pytest.mark.parametrize("timestamp", [None, "bad-timestamp", "2026-10-19T10:30:00"])
def test_invalid_canonical_timestamp_prevents_outputs(timestamp):
    predictor = FakePredictor(PredictorResult(prediction(), request={"occurredAt": timestamp}))
    output, training = FakeOutput(), FakeTraining()
    with pytest.raises(ValueError):
        handle_message(json.dumps(request_event()).encode(), predictor, output, training)
    assert output.items == []
    assert training.items == []
