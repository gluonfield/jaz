import json
import os

from confluent_kafka import Consumer, KafkaException

TOPIC = "learning.etl.records"


def load_idempotently(record):
    pass


consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": "etl-sink",
        "auto.offset.reset": "earliest",
        "enable.auto.commit": False,
    }
)
consumer.subscribe([TOPIC])

try:
    while True:
        message = consumer.poll(1.0)
        if message is None:
            continue
        if message.error():
            raise KafkaException(message.error())
        record = json.loads(message.value())
        load_idempotently(record)
        consumer.commit(message=message, asynchronous=False)
        print(f"loaded {record['record_id']}")
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
