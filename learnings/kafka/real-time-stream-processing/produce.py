import json
import os
import uuid

from confluent_kafka import KafkaException, Producer

TOPIC = "learning.stream.raw"


def delivery_report(error, _message):
    if error is not None:
        raise KafkaException(error)


event = {"event_id": str(uuid.uuid4()), "data": {}}
producer = Producer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "enable.idempotence": True,
    }
)
producer.produce(
    TOPIC,
    key=event["event_id"].encode(),
    value=json.dumps(event).encode(),
    on_delivery=delivery_report,
)

if producer.flush(10):
    raise TimeoutError("event was not delivered within 10 seconds")

print(event["event_id"])
