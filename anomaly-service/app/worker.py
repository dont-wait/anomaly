"""Kafka worker: TransferCreated -> EventStoreDB history -> GATv2 -> output + training."""
import logging
import time
from datetime import datetime, timezone

from prometheus_client import Counter, Histogram, Gauge, start_http_server

from .contracts import ContractError, build_risk_event
from .events import select_trigger_event
from .history import EventStoreHistory
from .predictor import LocalBundlePredictor
from .publisher import KafkaOutputPublisher
from .training import TrainingDataSink

log = logging.getLogger(__name__)

MESSAGES_TOTAL = Counter("ml_messages_total", "Kafka messages received", ["result"])
PREDICTIONS_TOTAL = Counter("ml_predictions_total", "Predictions published")
PROCESSING_SECONDS = Histogram("ml_processing_seconds", "Message processing time")
LAST_SUCCESS = Gauge("ml_last_success_timestamp_seconds", "Unix time of last successful message")


def handle_message(raw, predictor, output_publisher, training_sink):
    event = select_trigger_event(raw)
    if event is None:
        return None
    result = predictor.predict(event.event_id)
    prediction = result.prediction
    if prediction.transaction_id != event.transaction_id:
        raise ContractError("model response transaction_id differs from the Kafka trigger")
    raw_transaction_id = result.request.get("transactionId") or result.request.get("transaction_id")
    if raw_transaction_id and raw_transaction_id != event.transaction_id:
        raise ContractError("raw request transactionId differs from the Kafka trigger")
    risk_event = build_risk_event(event, prediction)
    output_publisher.publish(risk_event)
    training_sink.record(event, result.request, prediction)
    return risk_event


def run(config):
    from kafka import KafkaConsumer, KafkaProducer

    history = EventStoreHistory.from_uri(config.eventstore_uri, config.eventstore_stream)
    predictor = LocalBundlePredictor(
        config.model_bundle_path,
        history,
        gatv2_src_path=config.gatv2_src_path,
        device=config.model_device,
    )
    consumer = KafkaConsumer(
        config.input_topic,
        bootstrap_servers=list(config.kafka_brokers),
        group_id=config.consumer_group,
        enable_auto_commit=False,
        auto_offset_reset=config.auto_offset_reset,
        max_poll_records=1,
        max_poll_interval_ms=900_000,
    )
    producer = KafkaProducer(
        bootstrap_servers=list(config.kafka_brokers),
        acks="all",
        retries=3,
    )
    output = KafkaOutputPublisher(producer, config.output_topic,
                                  timeout=config.delivery_timeout_seconds)
    training = TrainingDataSink(
        config.training_data_path,
        topic=config.training_topic,
        fsync=config.training_data_fsync,
        producer=producer,
        timeout=config.delivery_timeout_seconds,
    )
    start_http_server(config.metrics_port)
    log.info("ML metrics listening on :%s", config.metrics_port)
    log.info("ML publisher started brokers=%s input=%s output=%s training=%s bundle=%s",
             config.kafka_brokers, config.input_topic, config.output_topic,
             config.training_topic, config.model_bundle_path)
    try:
        for message in consumer:
            started = time.monotonic()
            try:
                result = handle_message(message.value, predictor, output, training)
                consumer.commit()
                MESSAGES_TOTAL.labels("predicted" if result is not None else "ignored").inc()
                if result is not None:
                    PREDICTIONS_TOTAL.inc()
                    log.info("prediction published event_id=%s transaction_id=%s score=%.6f label=%d",
                             result["event_id"], result["transaction_id"],
                             result["risk_score"], result["predicted_label"])
                LAST_SUCCESS.set(datetime.now(timezone.utc).timestamp())
            except Exception:
                MESSAGES_TOTAL.labels("failed").inc()
                raise
            finally:
                PROCESSING_SECONDS.observe(time.monotonic() - started)
    finally:
        consumer.close()
        producer.close()
        predictor.close()
        log.info("ML publisher stopped")
