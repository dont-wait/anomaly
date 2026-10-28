"""UTC microsecond parsing shared by Kafka triggers and EventStoreDB history."""
import re
from datetime import datetime, timezone

_EXTRA_FRACTION = re.compile(r"(\.\d{6})\d+")


def microseconds(value):
    """Parse RFC3339/ISO-8601 with up to nanosecond precision into epoch microseconds."""
    if isinstance(value, datetime):
        parsed = value
    elif isinstance(value, str) and value.strip():
        text = _EXTRA_FRACTION.sub(r"\1", value.strip()).replace("Z", "+00:00")
        try:
            parsed = datetime.fromisoformat(text)
        except ValueError as error:
            raise ValueError(f"invalid timestamp: {value}") from error
    else:
        raise ValueError("timestamp must be a nonempty string")
    if parsed.tzinfo is None:
        raise ValueError("timestamp must include a timezone")
    return int(parsed.astimezone(timezone.utc).timestamp() * 1_000_000)
