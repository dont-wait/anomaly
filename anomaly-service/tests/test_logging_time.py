"""Regression tests for the UTC-vs-Vietnam log display boundary."""

import logging
from datetime import datetime, timezone

from app.logging_time import VietnamLogFormatter


def test_console_log_timestamp_is_vietnam_local_with_explicit_offset():
    formatter = VietnamLogFormatter("%(asctime)s %(levelname)s %(message)s")
    record = logging.LogRecord(
        "app.worker", logging.INFO, "worker.py", 1, "assessment published", (), None
    )
    record.created = datetime(2026, 11, 7, 2, 34, tzinfo=timezone.utc).timestamp()

    line = formatter.format(record)

    assert line == "2026-11-07T09:34:00+07:00 INFO assessment published"


def test_formatter_does_not_change_the_record_instant():
    formatter = VietnamLogFormatter("%(asctime)s %(message)s")
    instant = datetime(2026, 11, 7, 23, 45, tzinfo=timezone.utc)
    record = logging.LogRecord("app", logging.INFO, "__main__.py", 1, "ready", (), None)
    record.created = instant.timestamp()

    assert formatter.format(record).startswith("2026-11-08T06:45:00+07:00 ")
    assert record.created == instant.timestamp()
