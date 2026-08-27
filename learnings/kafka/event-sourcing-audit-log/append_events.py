import json
import os
import uuid

from confluent_kafka import KafkaException, Producer

TOPIC = "learning.event-sourcing.accounts"


def delivery_report(error, _message):
    if error is not None:
        raise KafkaException(error)


account_id = str(uuid.uuid4())
events = [
    {"type": "account.opened", "data": {}},
    {"type": "account.email-changed", "data": {}},
]
producer = Producer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "enable.idempotence": True,
    }
)

for event in events:
    envelope = {
        "event_id": str(uuid.uuid4()),
        "aggregate_id": account_id,
        **event,
    }
    producer.produce(
        TOPIC,
        key=account_id.encode(),
        value=json.dumps(envelope).encode(),
        on_delivery=delivery_report,
    )
    producer.poll(0)

if producer.flush(10):
    raise TimeoutError("events were not delivered within 10 seconds")

print(account_id)
