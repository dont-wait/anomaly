"""Kafka output publisher; the producer is injected so tests stay offline."""
import json


class KafkaOutputPublisher:
    def __init__(self, producer, topic, *, timeout=30):
        if not topic:
            raise ValueError("output topic is required")
        self.producer = producer
        self.topic = topic
        self.timeout = timeout

    def publish(self, risk_event):
        key = ":".join((
            risk_event["transaction_id"],
            risk_event["model_version"],
            risk_event["feature_version"],
        )).encode()
        value = json.dumps(risk_event, ensure_ascii=False, separators=(",", ":")).encode()
        self.producer.send(self.topic, key=key, value=value).get(timeout=self.timeout)
