"""Console entry point for the Anomaly ML publisher."""
import logging
import sys

from .config import ConfigError, load_config
from .worker import run


def main():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
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
