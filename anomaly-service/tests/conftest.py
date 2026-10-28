import json


def request_event(**overrides):
    event = {
        "event_id": "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70",
        "event_type": "TransferCreated",
        "stream": "banking",
        "stream_revision": 123,
        "occurred_at": "2026-10-19T10:30:00.123456789Z",
        "schema_version": 2,
        "payload": {
            "id": "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70",
            "type": "TransferCreated",
            "at": "2026-10-19T10:30:00.123456789Z",
            "transfer": {
                "ID": "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "Source": {"AccountNo": "ACC001"},
                "Destination": {"AccountNo": "ACC002"},
                "Amount": 50_000_000,
                "Channel": "mobile",
                "CreatedAt": "2026-10-19T10:30:00.123456789Z",
                "DeviceID": "DEVICE01",
                "Platform": "android",
                "Authentication": {"Method": "email_otp", "Verified": True},
            },
        },
    }
    event.update(overrides)
    return event


def request_event_bytes(**overrides):
    return json.dumps(request_event(**overrides)).encode()


def raw_bank_record(event_id=None, event_type="TransferCreated", occurred_at=None):
    """Canonical EventStoreDB Record.Event, with transfer metadata alongside."""
    transaction_id = "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
    event_id = event_id or "6d1f3f0e-5f4a-4d5f-9d1a-2b3c4d5e6f70"
    occurred_at = occurred_at or "2026-10-19T10:30:00.123456789Z"
    return {
        "id": event_id,
        "type": event_type,
        "at": occurred_at,
        "event": {
            "eventId": event_id,
            "transactionId": transaction_id,
            "eventType": event_type,
            "schemaVersion": 1,
            "sequence": 1 if event_type == "TransferCreated" else 2,
            "occurredAt": occurred_at,
            "payload": {
                "sourceAccountId": "ACC001",
                "destinationAccountId": "ACC002",
                "amount": 50_000_000,
                "fee": 0,
                "currency": "VND",
                "channel": "mobile",
                "status": "awaiting_otp" if event_type != "TransferCompleted" else "success",
            },
        },
        "transfer": {
            "ID": transaction_id,
            "Source": {"AccountNo": "ACC001"},
            "Destination": {"AccountNo": "ACC002"},
            "Amount": 50_000_000,
            "Channel": "mobile",
            "CreatedAt": occurred_at,
            "DeviceID": "DEVICE01",
            "Platform": "android",
            "Authentication": {"Method": "email_otp", "Verified": True},
        },
    }
