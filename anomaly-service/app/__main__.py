"""Console entry point for the Anomaly ML publisher."""
import logging
import sys

from .config import ConfigError, load_config
from .logging_time import VietnamLogFormatter
from .worker import run


def main():
    formatter = VietnamLogFormatter(
        fmt="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    handler = logging.StreamHandler()
    handler.setFormatter(formatter)
    logging.basicConfig(level=logging.INFO, handlers=[handler])
    log = logging.getLogger("app")
    try:
        from dotenv import load_dotenv
        load_dotenv()
        config = load_config()
    except ConfigError as error:
        log.error("invalid configuration: %s", error)
        return 2
    try:
        run(config)
    except KeyboardInterrupt:
        log.info("interrupted")
    except Exception:
        log.exception("ML publisher stopped")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
