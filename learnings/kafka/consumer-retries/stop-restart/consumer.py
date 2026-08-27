import json
import os

from confluent_kafka import Consumer, KafkaException

TOPIC = "learning.consumer-retries.input"


def handle_event(event):
    if os.getenv("FAIL_PROCESSING") == "1":
        raise RuntimeError(f"mock failure for {event['event_id']}")


consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": "retry-stop-restart",
        "auto.offset.reset": "earliest",
        "enable.auto.commit": False,
    }
)
consumer.subscribe([TOPIC])

try:
    while True:
        message = consumer.poll(1)
        if message is None:
            continue
        if message.error():
            raise KafkaException(message.error())

        event = json.loads(message.value())
        handle_event(event)
        consumer.commit(message=message, asynchronous=False)
        print(f"committed offset {message.offset()}")
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
