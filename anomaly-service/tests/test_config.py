import os
from pathlib import Path
from unittest.mock import patch

import pytest

from app.config import ConfigError, load_config


def test_defaults_match_anomaly_topics_and_local_bundle():
    with patch.dict(os.environ, {}, clear=True):
        config = load_config()
    assert config.kafka_brokers == ("localhost:9092",)
    assert config.input_topic == "anomaly.bank.events.v1"
    assert config.output_topic == "anomaly.bank.risk-decisions.v1"
    assert config.training_topic == "anomaly.model.training.v1"
    assert config.consumer_group == "ml-pipeline"
    assert config.auto_offset_reset == "earliest"
    assert config.model_bundle_path.endswith("models/gatv2-1m-baseline-v1")
    assert (Path(config.model_bundle_path) / "bundle.json").is_file()
    assert config.gatv2_src_path.endswith("GATv2/src")
    assert config.model_device == "cpu"
    assert config.eventstore_stream == "banking"
    assert config.training_data_fsync is True
    assert config.training_data_max_bytes == 100 * 1024 * 1024
    assert config.training_data_backup_count == 5


def test_environment_overrides():
    env = {
        "KAFKA_BROKERS": "kafka-a:9092, kafka-b:9092",
        "KAFKA_INPUT_TOPIC": "events.v1",
        "KAFKA_OUTPUT_TOPIC": "risk.v1",
        "KAFKA_TRAINING_TOPIC": "training.v1",
        "KAFKA_CONSUMER_GROUP": "ml-test",
        "KAFKA_AUTO_OFFSET_RESET": "latest",
        "KAFKA_DELIVERY_TIMEOUT_SECONDS": "5",
        "MODEL_BUNDLE_PATH": "/models/gatv2",
        "GATV2_SRC_PATH": "/src/GATv2/src",
        "MODEL_DEVICE": "cuda",
        "EVENTSTORE_URI": "esdb://eventstore:2113?tls=false",
        "EVENTSTORE_STREAM": "banking",
        "TRAINING_DATA_PATH": "/tmp/training.jsonl",
        "TRAINING_DATA_FSYNC": "false",
        "TRAINING_DATA_MAX_BYTES": "1024",
        "TRAINING_DATA_BACKUP_COUNT": "2",
    }
    with patch.dict(os.environ, env, clear=True):
        config = load_config()
    assert config.kafka_brokers == ("kafka-a:9092", "kafka-b:9092")
    assert config.input_topic == "events.v1"
    assert config.output_topic == "risk.v1"
    assert config.training_topic == "training.v1"
    assert config.consumer_group == "ml-test"
    assert config.auto_offset_reset == "latest"
    assert config.delivery_timeout_seconds == 5
    assert config.model_bundle_path == "/models/gatv2"
    assert config.gatv2_src_path == "/src/GATv2/src"
    assert config.model_device == "cuda"
    assert config.eventstore_uri == "esdb://eventstore:2113?tls=false"
    assert config.training_data_path == "/tmp/training.jsonl"
    assert config.training_data_fsync is False
    assert config.training_data_max_bytes == 1024
    assert config.training_data_backup_count == 2


def test_relative_gatv2_source_can_be_disabled():
    with patch.dict(os.environ, {"GATV2_SRC_PATH": ""}, clear=True):
        assert load_config().gatv2_src_path is None


def test_topic_conflicts_are_rejected():
    for env in (
        {"KAFKA_INPUT_TOPIC": "same", "KAFKA_OUTPUT_TOPIC": "same"},
        {"KAFKA_INPUT_TOPIC": "same", "KAFKA_OUTPUT_TOPIC": "risk", "KAFKA_TRAINING_TOPIC": "same"},
        {
            "KAFKA_INPUT_TOPIC": "events",
            "KAFKA_OUTPUT_TOPIC": "risk",
            "KAFKA_TRAINING_TOPIC": "risk",
        },
    ):
        with patch.dict(os.environ, env, clear=True), pytest.raises(ConfigError):
            load_config()


def test_invalid_values_are_rejected():
    for env in (
        {"KAFKA_AUTO_OFFSET_RESET": "middle"},
        {"TRAINING_DATA_FSYNC": "maybe"},
        {"KAFKA_DELIVERY_TIMEOUT_SECONDS": "-1"},
        {"MODEL_BUNDLE_PATH": ""},
        {"EVENTSTORE_URI": ""},
        {"EVENTSTORE_STREAM": ""},
        {"TRAINING_DATA_PATH": ""},
        {"TRAINING_DATA_MAX_BYTES": "0"},
        {"TRAINING_DATA_MAX_BYTES": "invalid"},
        {"TRAINING_DATA_BACKUP_COUNT": "0"},
    ):
        with patch.dict(os.environ, env, clear=True), pytest.raises(ConfigError):
            load_config()
