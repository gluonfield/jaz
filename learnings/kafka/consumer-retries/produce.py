import json
import os
import uuid

from confluent_kafka import KafkaException, Producer

TOPIC = "learning.consumer-retries.input"


def delivery_report(error, message):
    if error:
        raise KafkaException(error)
    print(
        f"produced {message.key().decode()} to {message.topic()}[{message.partition()}]"
    )


event = {
    "event_id": str(uuid.uuid4()),
    "attempt": 0,
    "failures_before_success": int(os.getenv("FAILURES_BEFORE_SUCCESS", "2")),
}
producer = Producer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "enable.idempotence": True,
    }
)
producer.produce(
    TOPIC,
    key=event["event_id"],
    value=json.dumps(event),
    on_delivery=delivery_report,
)
if producer.flush(10):
    raise TimeoutError("record was not delivered within 10 seconds")
