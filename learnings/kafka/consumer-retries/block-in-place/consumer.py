import json
import os
import time

from confluent_kafka import Consumer, KafkaException

TOPIC = "learning.consumer-retries.input"
MAX_ATTEMPTS = 3


class TransientError(Exception):
    pass


def handle_event(event, attempt):
    if attempt <= event["failures_before_success"]:
        raise TransientError("mock transient failure")


consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": "retry-block-in-place",
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
        for attempt in range(1, MAX_ATTEMPTS + 1):
            try:
                handle_event(event, attempt)
            except TransientError:
                print(f"attempt {attempt} failed at offset {message.offset()}")
                if attempt == MAX_ATTEMPTS:
                    raise
                time.sleep(2 ** (attempt - 1))
            else:
                consumer.commit(message=message, asynchronous=False)
                print(f"committed offset {message.offset()} after attempt {attempt}")
                break
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
