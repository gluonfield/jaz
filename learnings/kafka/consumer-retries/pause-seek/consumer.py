import json
import os
import time

from confluent_kafka import Consumer, KafkaException, TopicPartition

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
        "group.id": "retry-pause-seek",
        "auto.offset.reset": "earliest",
        "enable.auto.commit": False,
    }
)
consumer.subscribe([TOPIC])
attempts = {}
pending = {}

try:
    while True:
        now = time.monotonic()
        assigned = {(item.topic, item.partition) for item in consumer.assignment()}
        for partition_key, (offset, retry_at) in list(pending.items()):
            if retry_at > now:
                continue
            record_key = (*partition_key, offset)
            if partition_key not in assigned:
                attempts.pop(record_key, None)
                pending.pop(partition_key)
                continue
            position = TopicPartition(*partition_key, offset)
            consumer.seek(position)
            consumer.resume([position])
            pending.pop(partition_key)
            print(f"resumed {partition_key} at offset {offset}")

        message = consumer.poll(0.5)
        if message is None:
            continue
        if message.error():
            raise KafkaException(message.error())

        record_key = (message.topic(), message.partition(), message.offset())
        attempt = attempts.get(record_key, 0) + 1
        event = json.loads(message.value())
        try:
            handle_event(event, attempt)
        except TransientError:
            print(f"attempt {attempt} failed at offset {message.offset()}")
            if attempt == MAX_ATTEMPTS:
                raise
            partition_key = (message.topic(), message.partition())
            attempts[record_key] = attempt
            pending[partition_key] = (
                message.offset(),
                time.monotonic() + 2 ** (attempt - 1),
            )
            consumer.pause([TopicPartition(*partition_key)])
            continue

        consumer.commit(message=message, asynchronous=False)
        attempts.pop(record_key, None)
        print(f"committed offset {message.offset()} after attempt {attempt}")
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
