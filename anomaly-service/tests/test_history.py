import json

import pytest
from conftest import raw_bank_record

from app.history import EventStoreHistory, HistoryError


class Record:
    def __init__(self, data):
        self.data = json.dumps(data).encode()


class Client:
    def __init__(self, records):
        self.records = records
        self.closed = False

    def get_stream(self, stream):
        assert stream == "banking"
        return self.records

    def close(self):
        self.closed = True


def transfer_record(event_id, event_type, occurred_at, transaction_id):
    data = raw_bank_record(event_id=event_id, event_type=event_type, occurred_at=occurred_at)
    data["event"]["transactionId"] = transaction_id
    data["transfer"]["ID"] = transaction_id
    return data


def test_reads_request_and_strictly_earlier_transfer_events():
    created_id = "44444444-4444-4444-8444-444444444444"
    earlier = transfer_record(
        "11111111-1111-4111-8111-111111111111", "TransferCompleted",
        "2026-10-19T09:00:00Z", "aaaaaaa0-aaaa-4aaa-8aaa-aaaaaaaaaaa0",
    )
    same_time = transfer_record(
        "22222222-2222-4222-8222-222222222222", "TransferCompleted",
        "2026-10-19T10:30:00.123456789Z", "aaaaaaa2-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
    )
    created = transfer_record(
        created_id, "TransferCreated",
        "2026-10-19T10:30:00.123456789Z", "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
    )
    future = transfer_record(
        "33333333-3333-4333-8333-333333333333", "TransferCompleted",
        "2026-10-19T11:00:00Z", "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
    )
    account_event = {"id": "event-x", "type": "BankAccountCreated", "at": "2026-10-19T08:00:00Z"}
    client = Client([Record(account_event), Record(earlier), Record(same_time),
                     Record(created), Record(future)])
    history = EventStoreHistory(client, "banking")

    request, prefix = history.read_request_and_prefix(created_id)

    assert request["eventId"] == created_id
    assert request["eventType"] == "TransferCreated"
    assert request["payload"]["device_id"] == "DEVICE01"
    assert request["payload"]["platform"] == "android"
    assert request["payload"]["authentication_method"] == "otp"
    assert request["payload"]["authentication_verified"] is True
    assert [event["eventId"] for event in prefix] == [earlier["id"]]
    assert "device_id" not in prefix[0]["payload"]
    history.close()
    assert client.closed is True


def test_missing_request_raises():
    client = Client([Record(raw_bank_record())])
    history = EventStoreHistory(client, "banking")
    with pytest.raises(HistoryError, match="not found"):
        history.read_request_and_prefix("ffffffff-ffff-4fff-8fff-ffffffffffff")


def test_non_request_trigger_raises():
    event_id = "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70"
    record = transfer_record(
        event_id, "TransferCompleted",
        "2026-10-19T10:31:00Z", "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
    )
    client = Client([Record(record)])
    history = EventStoreHistory(client, "banking")
    with pytest.raises(HistoryError, match="not a TransferCreated"):
        history.read_request_and_prefix(event_id)


def test_request_preserves_client_ip_without_mutating_stored_event():
    record = raw_bank_record()
    record["transfer"]["ClientIP"] = "203.0.113.10"
    history = EventStoreHistory(Client([Record(record)]), "banking")
    request, _ = history.read_request_and_prefix(record["id"])
    assert request["payload"]["ip_address"] == "203.0.113.10"
    assert "ip_address" not in record["event"]["payload"]
