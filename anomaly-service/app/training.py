"""Persist training samples to bounded JSONL files and optionally a Kafka topic.

JSONL writers sharing a path must run on a filesystem supporting POSIX flock.
"""

import fcntl
import json
import os
from pathlib import Path

from .contracts import build_training_record


class TrainingDataSink:
    """At-least-once recorder; duplicate records are possible after a retry."""

    def __init__(
        self,
        path,
        *,
        topic=None,
        fsync=True,
        producer=None,
        timeout=30,
        max_bytes=100 * 1024 * 1024,
        backup_count=5,
    ):
        if max_bytes <= 0 or backup_count < 1:
            raise ValueError("max_bytes must be positive and backup_count at least 1")
        self.path = Path(path)
        self.topic = topic
        self.fsync = fsync
        self.producer = producer
        self.timeout = timeout
        self.max_bytes = max_bytes
        self.backup_count = backup_count

    def record(self, event, request, prediction, *, producer=None, timeout=None):
        producer = self.producer if producer is None else producer
        if self.topic and producer is None:
            raise ValueError("a Kafka producer is required for the training topic")
        record = build_training_record(event, request, prediction)
        self._append(record)
        if self.topic:
            value = json.dumps(record, ensure_ascii=False, separators=(",", ":")).encode()
            producer.send(self.topic, key=record["prediction_key"].encode(), value=value).get(
                timeout=self.timeout if timeout is None else timeout
            )
        return record

    def _append(self, record):
        line = (json.dumps(record, ensure_ascii=False, separators=(",", ":")) + "\n").encode()
        if len(line) > self.max_bytes:
            raise ValueError("training record exceeds TRAINING_DATA_MAX_BYTES")
        self.path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        # A stable lock file serializes repair/rotation/append, even after rename.
        lock_fd = os.open(str(self.path) + ".lock", os.O_WRONLY | os.O_CREAT, 0o600)
        with os.fdopen(lock_fd, "wb") as lock:
            os.fchmod(lock.fileno(), 0o600)
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
            self._repair_tail()
            if self.path.stat().st_size + len(line) > self.max_bytes:
                self._rotate()
            fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
            with os.fdopen(fd, "ab") as stream:
                os.fchmod(stream.fileno(), 0o600)
                stream.write(line)
                stream.flush()
                if self.fsync:
                    os.fsync(stream.fileno())

    def _repair_tail(self):
        fd = os.open(self.path, os.O_RDWR | os.O_CREAT, 0o600)
        with os.fdopen(fd, "r+b") as stream:
            os.fchmod(stream.fileno(), 0o600)
            end = stream.seek(0, os.SEEK_END)
            if not end:
                return
            stream.seek(end - 1)
            if stream.read(1) == b"\n":
                return
            # Only inspect the unfinished final line; never load the whole file.
            position = end
            chunks = []
            while position:
                start = max(0, position - 65536)
                stream.seek(start)
                block = stream.read(position - start)
                newline = block.rfind(b"\n")
                if newline >= 0:
                    chunks.append(block[newline + 1 :])
                    position = start + newline + 1
                    break
                chunks.append(block)
                position = start
            tail = b"".join(reversed(chunks))
            try:
                json.loads(tail)
            except (ValueError, UnicodeError):
                # Drop an interrupted fragment so every remaining line is JSON.
                stream.truncate(position)
            else:
                # A complete last record without a newline must be retained.
                stream.seek(0, os.SEEK_END)
                stream.write(b"\n")
            stream.flush()
            if self.fsync:
                os.fsync(stream.fileno())

    def _rotate(self):
        for index in range(self.backup_count, 0, -1):
            source = self.path if index == 1 else Path(f"{self.path}.{index - 1}")
            if source.exists():
                source.replace(f"{self.path}.{index}")
