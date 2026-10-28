import json

import pytest
from conftest import request_event, request_event_bytes

from app.events import InputError, select_trigger_event
from app.timeutil import microseconds


def test_select_transfer_created_trigger():
    event = select_trigger_event(request_event_bytes())
    assert event is not None
    assert event.event_id == "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70"
    assert event.transaction_id == "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
    assert event.event_type == "TransferCreated"
    assert event.stream_revision == 123
    assert event.occurred_at_us == microseconds("2026-10-19T10:30:00.123456789Z")
    assert event.occurred_at_us % 1_000_000 == 123_456


def test_other_events_are_ignored():
    for event_type in (
        "TransferValidate",
        "TransferCompleted",
        "TransferCancelled",
        "TransferOTPUpdated",
        "TestEvent",
    ):
        payload = request_event(event_type=event_type)
        assert select_trigger_event(json.dumps(payload).encode()) is None


def test_trigger_requires_transfer_snapshot():
    payload = request_event()
    payload["payload"] = {}
    with pytest.raises(InputError, match="transfer snapshot"):
        select_trigger_event(json.dumps(payload).encode())


def test_invalid_json_and_timestamp_are_rejected():
    with pytest.raises(InputError, match="valid JSON"):
        select_trigger_event(b"not-json")
    payload = request_event(occurred_at="not-a-time")
    with pytest.raises(InputError, match="invalid timestamp"):
        select_trigger_event(json.dumps(payload).encode())
