"""Persist training samples to JSONL and optionally a Kafka topic."""
import json
import os
from pathlib import Path

from .contracts import build_training_record


class TrainingDataSink:
    """At-least-once recorder; duplicate records are possible after a retry."""

    def __init__(self, path, *, topic=None, fsync=True, producer=None, timeout=30):
        self.path = Path(path)
        self.topic = topic
        self.fsync = fsync
        self.producer = producer
        self.timeout = timeout

    def record(self, event, request, prediction, *, producer=None, timeout=None):
        producer = self.producer if producer is None else producer
        if self.topic and producer is None:
            raise ValueError("a Kafka producer is required for the training topic")
        record = build_training_record(event, request, prediction)
        self._append(record)
        if self.topic:
            value = json.dumps(record, ensure_ascii=False, separators=(",", ":")).encode()
            producer.send(self.topic, key=record["prediction_key"].encode(),
                          value=value).get(timeout=self.timeout if timeout is None else timeout)
        return record

    def _append(self, record):
        self.path.parent.mkdir(parents=True, exist_ok=True)
        line = json.dumps(record, ensure_ascii=False, separators=(",", ":")) + "\n"
        with self.path.open("a", encoding="utf-8") as stream:
            stream.write(line)
            stream.flush()
            if self.fsync:
                os.fsync(stream.fileno())
