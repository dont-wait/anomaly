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
    prediction = Prediction.from_payload(
        {
            "transaction_id": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
            "risk_score": 0.9821,
            "predicted_label": 1,
            "threshold": 0.9517,
            "model_version": "gatv2-device-control-v1",
            "feature_version": "schema-3-test",
        }
    )
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
    sink = TrainingDataSink(
        tmp_path / "predictions.jsonl", topic="training", fsync=False, producer=producer, timeout=9
    )
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


def test_append_recovers_from_interrupted_partial_line(tmp_path):
    path = tmp_path / "predictions.jsonl"
    # Write a corrupted partial line without ending newline
    path.write_bytes(b'{"partial": "json"')

    sink = TrainingDataSink(path, fsync=False)
    event, request, prediction = sample()
    sink.record(event, request, prediction)

    lines = path.read_text(encoding="utf-8").splitlines()
    assert len(lines) == 1
    parsed = json.loads(lines[0])
    assert parsed["event"]["event_id"] == event.event_id


@pytest.mark.parametrize("tail", [b'{"partial":', b'{"text":"\xe1\xbb'])
def test_tail_repair_preserves_prior_complete_records(tmp_path, tail):
    path = tmp_path / "training.jsonl"
    path.write_bytes(b'{"prior":true}\n' + tail)
    TrainingDataSink(path, fsync=False)._append({"next": True})
    assert [json.loads(line) for line in path.read_bytes().splitlines()] == [
        {"prior": True},
        {"next": True},
    ]


def test_complete_record_missing_newline_is_kept(tmp_path):
    path = tmp_path / "training.jsonl"
    path.write_bytes(b'{"prior":true}')
    TrainingDataSink(path, fsync=False)._append({"next": True})
    assert [json.loads(line) for line in path.read_bytes().splitlines()] == [
        {"prior": True},
        {"next": True},
    ]


def test_tail_repair_crosses_read_chunk_boundary(tmp_path):
    path = tmp_path / "training.jsonl"
    path.write_bytes(b'{"prior":true}\n{"partial":"' + b"x" * 70000)
    TrainingDataSink(path, fsync=False)._append({"next": True})
    assert [json.loads(line) for line in path.read_bytes().splitlines()] == [
        {"prior": True},
        {"next": True},
    ]


def test_rotation_bounds_retained_files_and_keeps_valid_jsonl(tmp_path):
    path = tmp_path / "training.jsonl"
    sink = TrainingDataSink(path, max_bytes=16, backup_count=2, fsync=False)
    for i in range(6):
        sink._append({"i": i})
    assert [json.loads(line) for line in path.read_bytes().splitlines()] == [{"i": 4}, {"i": 5}]
    assert [
        json.loads(line) for line in path.with_suffix(".jsonl.1").read_bytes().splitlines()
    ] == [
        {"i": 2},
        {"i": 3},
    ]
    assert [
        json.loads(line) for line in path.with_suffix(".jsonl.2").read_bytes().splitlines()
    ] == [
        {"i": 0},
        {"i": 1},
    ]
    sink._append({"i": 6})
    assert len(list(tmp_path.glob("training.jsonl.*"))) == 3  # Two backups plus lock.
    assert all(file.stat().st_size <= 16 for file in tmp_path.iterdir())
    assert json.loads(path.read_text()) == {"i": 6}
    assert json.loads(path.with_suffix(".jsonl.2").read_text().splitlines()[0]) == {"i": 2}


def test_oversized_record_does_not_destroy_existing_data(tmp_path):
    path = tmp_path / "training.jsonl"
    sink = TrainingDataSink(path, max_bytes=16, fsync=False)
    sink._append({"prior": True})
    before = path.read_bytes()
    with pytest.raises(ValueError, match="exceeds"):
        sink._append({"too_big": "x" * 32})
    assert path.read_bytes() == before


def test_training_files_restrict_existing_permissions(tmp_path):
    path = tmp_path / "training.jsonl"
    path.touch(mode=0o666)
    path.chmod(0o666)
    TrainingDataSink(path, fsync=False)._append({"private": True})
    assert path.stat().st_mode & 0o777 == 0o600
    assert (tmp_path / "training.jsonl.lock").stat().st_mode & 0o777 == 0o600


def test_new_training_directory_is_private(tmp_path):
    path = tmp_path / "private" / "training.jsonl"
    TrainingDataSink(path, fsync=False)._append({"private": True})
    assert path.parent.stat().st_mode & 0o777 == 0o700


def test_concurrent_processes_keep_complete_records(tmp_path):
    import os
    import subprocess
    import sys
    from pathlib import Path

    path = tmp_path / "training.jsonl"
    script = """
import sys
from app.training import TrainingDataSink
sink = TrainingDataSink(sys.argv[1], fsync=False)
for index in range(25):
    sink._append({"writer": int(sys.argv[2]), "index": index})
"""
    env = {**os.environ, "PYTHONPATH": str(Path(__file__).resolve().parents[1])}
    processes = [
        subprocess.Popen([sys.executable, "-c", script, str(path), str(writer)], env=env)
        for writer in range(4)
    ]
    for process in processes:
        assert process.wait(timeout=15) == 0
    records = [json.loads(line) for line in path.read_bytes().splitlines()]
    assert len(records) == 100
    assert {(item["writer"], item["index"]) for item in records} == {
        (writer, index) for writer in range(4) for index in range(25)
    }
