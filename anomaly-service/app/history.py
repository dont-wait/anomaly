"""Raw banking history from EventStoreDB for GATv2 replay.

The relay's Kafka payload is sanitized and pseudonymizes account/device IDs.
GATv2 needs the raw journal to reconstruct the graph, so this reader consumes
the canonical `banking` stream directly.
"""
import json

from .timeutil import microseconds

TRANSFER_EVENT_TYPES = {
    "TransferCreated",
    "TransferCompleted",
    "TransferCancelled",
    "TransferOTPUpdated",
}


class HistoryError(RuntimeError):
    pass


class EventStoreHistory:
    def __init__(self, client, stream="banking"):
        if not stream:
            raise ValueError("EventStoreDB stream is required")
        self.client = client
        self.stream = stream

    @classmethod
    def from_uri(cls, uri, stream="banking"):
        if not uri:
            raise ValueError("EventStoreDB URI is required")
        from esdbclient import EventStoreDBClient
        return cls(EventStoreDBClient(uri=uri), stream)

    def read_request_and_prefix(self, trigger_event_id):
        """Return the raw TransferCreated request and all transfer events before it."""
        events = []
        request = None
        for record in self.client.get_stream(self.stream):
            data = _decode(record.data)
            if data.get("type") not in TRANSFER_EVENT_TYPES:
                continue
            event = data.get("event")
            if not isinstance(event, dict):
                continue
            event = _with_request_context(event, data.get("transfer"))
            at = microseconds(event.get("occurredAt"))
            events.append((at, event))
            if event.get("eventId") == trigger_event_id:
                request = (at, event)
        if request is None:
            raise HistoryError(f"request event {trigger_event_id} was not found in {self.stream}")

        request_at, request_event = request
        if request_event.get("eventType") != "TransferCreated":
            raise HistoryError(f"event {trigger_event_id} is not a TransferCreated request")
        prefix = [
            event for at, event in sorted(events, key=lambda item: item[0])
            if at < request_at
        ]
        return request_event, prefix

    def close(self):
        close = getattr(self.client, "close", None)
        if close is not None:
            close()


def _decode(data):
    if isinstance(data, dict):
        return data
    if isinstance(data, (bytes, bytearray)):
        data = data.decode()
    if not isinstance(data, str):
        raise HistoryError("EventStoreDB record data must be JSON")
    try:
        value = json.loads(data)
    except ValueError as error:
        raise HistoryError("EventStoreDB record is not valid JSON") from error
    if not isinstance(value, dict):
        raise HistoryError("EventStoreDB record must be an object")
    return value


def _with_request_context(event, transfer):
    """Copy optional device/auth metadata into a request event payload."""
    if event.get("eventType") != "TransferCreated":
        return event
    if not isinstance(transfer, dict):
        return event
    payload = dict(event.get("payload") or {})
    device_id = transfer.get("DeviceID")
    if isinstance(device_id, str) and device_id.strip():
        payload.setdefault("device_id", device_id.strip())
    platform = transfer.get("Platform")
    if isinstance(platform, str) and platform.strip():
        payload.setdefault("platform", platform.strip().lower())
    authentication = transfer.get("Authentication")
    if isinstance(authentication, dict):
        method = authentication.get("Method")
        if isinstance(method, str) and method.strip():
            method = method.strip().lower().replace("-", "_")
            if method == "email_otp":
                method = "otp"
            payload.setdefault("authentication_method", method)
        verified = authentication.get("Verified")
        if isinstance(verified, bool):
            payload.setdefault("authentication_verified", verified)
    return {**event, "payload": payload}
