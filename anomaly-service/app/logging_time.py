"""Vietnam-local presentation timestamps for anomaly-service console logs.

Event/Kafka/ML timestamps deliberately remain in UTC; only the log prefix
uses UTC+07:00 to match the operations team's wall clock.
"""

import logging
from datetime import datetime, timedelta, timezone

_VIETNAM_TZ = timezone(timedelta(hours=7))


class VietnamLogFormatter(logging.Formatter):
    """Render the log record instant with an explicit +07:00 UTC offset."""

    def formatTime(self, record, datefmt=None):  # noqa: N802 - stdlib method name
        when = datetime.fromtimestamp(record.created, tz=_VIETNAM_TZ)
        if datefmt:
            return when.strftime(datefmt)
        return when.isoformat(timespec="seconds")
