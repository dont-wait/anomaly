"""Environment configuration for the local GATv2 publisher."""

import os
from dataclasses import dataclass
from pathlib import Path


class ConfigError(ValueError):
    pass


@dataclass(frozen=True)
class Config:
    kafka_brokers: tuple[str, ...]
    input_topic: str
    output_topic: str
    training_topic: str | None
    consumer_group: str
    auto_offset_reset: str
    delivery_timeout_seconds: float
    model_bundle_path: str
    gatv2_src_path: str | None
    model_device: str
    eventstore_uri: str
    eventstore_stream: str
    training_data_path: str
    training_data_fsync: bool
    metrics_port: int
    training_data_max_bytes: int = 100 * 1024 * 1024
    training_data_backup_count: int = 5


def split_csv(value):
    return tuple(part.strip() for part in value.split(",") if part.strip())


def env(name, default):
    value = os.getenv(name)
    return default if value is None else value


def env_bool(name, default):
    value = os.getenv(name)
    if value is None or value.strip() == "":
        return default
    normalized = value.strip().lower()
    if normalized in {"1", "true", "yes", "on"}:
        return True
    if normalized in {"0", "false", "no", "off"}:
        return False
    raise ConfigError(f"{name} must be a boolean")


def env_number(name, default, cast):
    value = os.getenv(name)
    if value is None or value.strip() == "":
        return default
    try:
        return cast(value)
    except ValueError as error:
        raise ConfigError(f"{name} must be a valid number") from error


def load_config():
    brokers = split_csv(env("KAFKA_BROKERS", "localhost:9092"))
    input_topic = env("KAFKA_INPUT_TOPIC", "anomaly.bank.events.v1").strip()
    output_topic = env("KAFKA_OUTPUT_TOPIC", "anomaly.bank.risk-decisions.v1").strip()
    training_topic = env("KAFKA_TRAINING_TOPIC", "anomaly.model.training.v1").strip() or None
    consumer_group = env("KAFKA_CONSUMER_GROUP", "ml-pipeline").strip()
    auto_offset_reset = env("KAFKA_AUTO_OFFSET_RESET", "earliest").strip().lower()
    model_bundle_path = env(
        "MODEL_BUNDLE_PATH",
        str(Path(__file__).resolve().parent.parent / "models" / "gatv2-1m-baseline-v1"),
    ).strip()
    gatv2_src_path = env("GATV2_SRC_PATH", "../../GATv2/src").strip() or None
    model_device = env("MODEL_DEVICE", "cpu").strip()
    eventstore_uri = env("EVENTSTORE_URI", "esdb://localhost:2113?tls=false").strip()
    eventstore_stream = env("EVENTSTORE_STREAM", "banking").strip()
    training_data_path = env("TRAINING_DATA_PATH", "./data/training/predictions.jsonl").strip()
    metrics_port = env_number("METRICS_PORT", 9100, int)

    training_data_max_bytes = env_number("TRAINING_DATA_MAX_BYTES", 100 * 1024 * 1024, int)
    training_data_backup_count = env_number("TRAINING_DATA_BACKUP_COUNT", 5, int)
    if training_data_max_bytes <= 0 or training_data_backup_count < 1:
        raise ConfigError("training data max bytes must be positive and backup count at least 1")

    if not brokers:
        raise ConfigError("KAFKA_BROKERS is required")
    for topic in (input_topic, output_topic, consumer_group):
        if not topic:
            raise ConfigError("Kafka input/output topics and consumer group are required")
    if input_topic == output_topic:
        raise ConfigError("Kafka input and output topics must differ")
    if training_topic in {input_topic, output_topic}:
        raise ConfigError("Kafka training topic must differ from input/output topics")
    for name, value in (
        ("MODEL_BUNDLE_PATH", model_bundle_path),
        ("EVENTSTORE_URI", eventstore_uri),
        ("EVENTSTORE_STREAM", eventstore_stream),
        ("TRAINING_DATA_PATH", training_data_path),
    ):
        if not value:
            raise ConfigError(f"{name} is required")
    if auto_offset_reset not in {"earliest", "latest"}:
        raise ConfigError("KAFKA_AUTO_OFFSET_RESET must be earliest or latest")

    delivery_timeout = env_number("KAFKA_DELIVERY_TIMEOUT_SECONDS", 30.0, float)
    if delivery_timeout <= 0:
        raise ConfigError("KAFKA_DELIVERY_TIMEOUT_SECONDS must be positive")
    if not 1 <= metrics_port <= 65535:
        raise ConfigError("METRICS_PORT must be between 1 and 65535")

    return Config(
        kafka_brokers=brokers,
        input_topic=input_topic,
        output_topic=output_topic,
        training_topic=training_topic,
        consumer_group=consumer_group,
        auto_offset_reset=auto_offset_reset,
        delivery_timeout_seconds=delivery_timeout,
        model_bundle_path=model_bundle_path,
        gatv2_src_path=gatv2_src_path,
        model_device=model_device,
        eventstore_uri=eventstore_uri,
        eventstore_stream=eventstore_stream,
        training_data_path=training_data_path,
        training_data_fsync=env_bool("TRAINING_DATA_FSYNC", True),
        metrics_port=metrics_port,
        training_data_max_bytes=training_data_max_bytes,
        training_data_backup_count=training_data_backup_count,
    )
