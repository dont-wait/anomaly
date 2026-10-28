"""Select request-time events from the sanitized Anomaly Kafka envelope.

Kafka is only a trigger. Account/device IDs in the relay payload are
pseudonymized and the GATv2 graph needs raw identities, so the publisher reads
the original request and its history from EventStoreDB instead.
"""
import json
from dataclasses import dataclass

from .timeutil import microseconds

TRIGGER_EVENT_TYPE = "TransferCreated"


class InputError(ValueError):
    """A trigger event cannot be converted into a Kafka trigger."""


@dataclass(frozen=True)
class KafkaEvent:
    event_id: str
    event_type: str
    stream: str
    stream_revision: int
    occurred_at: str
    occurred_at_us: int
    transaction_id: str
    schema_version: int


def select_trigger_event(raw):
    """Return None for other events; raise InputError for a malformed trigger."""
    try:
        envelope = json.loads(raw)
    except (TypeError, ValueError) as error:
        raise InputError("Kafka value is not valid JSON") from error
    if not isinstance(envelope, dict):
        raise InputError("Kafka value must be an object")
    if envelope.get("event_type") != TRIGGER_EVENT_TYPE:
        return None

    payload = envelope.get("payload")
    transfer = payload.get("transfer") if isinstance(payload, dict) else None
    if not isinstance(transfer, dict):
        raise InputError("request transfer snapshot is missing")
    transaction_id = _text(transfer.get("ID"), "transaction ID")
    occurred_at = _text(envelope.get("occurred_at"), "occurred_at")
    try:
        occurred_at_us = microseconds(occurred_at)
    except ValueError as error:
        raise InputError(str(error)) from error

    return KafkaEvent(
        event_id=_text(envelope.get("event_id"), "event_id"),
        event_type=TRIGGER_EVENT_TYPE,
        stream=_text(envelope.get("stream"), "stream"),
        stream_revision=_integer(envelope.get("stream_revision"), "stream_revision"),
        occurred_at=occurred_at,
        occurred_at_us=occurred_at_us,
        transaction_id=transaction_id,
        schema_version=_integer(envelope.get("schema_version"), "schema_version"),
    )


def _text(value, name):
    if not isinstance(value, str) or not value.strip():
        raise InputError(f"{name} is missing")
    return value.strip()


def _integer(value, name):
    if isinstance(value, bool):
        raise InputError(f"{name} must be an integer")
    if isinstance(value, int):
        return value
    if isinstance(value, float) and value.is_integer():
        return int(value)
    raise InputError(f"{name} must be an integer")
