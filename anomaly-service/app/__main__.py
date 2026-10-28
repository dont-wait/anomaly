"""Console entry point for the Anomaly ML publisher."""
import logging
import sys
import time

from .config import ConfigError, load_config
from .worker import run


def main():
    formatter = logging.Formatter(
        fmt="%(asctime)sZ %(levelname)s %(name)s %(message)s",
        datefmt="%Y-%m-%dT%H:%M:%S",
    )
    formatter.converter = time.gmtime
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
