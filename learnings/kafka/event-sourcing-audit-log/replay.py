import json
import os

from confluent_kafka import (
    OFFSET_BEGINNING,
    Consumer,
    KafkaException,
    TopicPartition,
)

TOPIC = "learning.event-sourcing.accounts"


def apply_event(event):
    pass


consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": "account-replay",
        "enable.auto.commit": False,
    }
)
metadata = consumer.list_topics(TOPIC, timeout=10)
consumer.assign(
    [
        TopicPartition(TOPIC, partition, OFFSET_BEGINNING)
        for partition in metadata.topics[TOPIC].partitions
    ]
)

try:
    while True:
        message = consumer.poll(1.0)
        if message is None:
            continue
        if message.error():
            raise KafkaException(message.error())
        event = json.loads(message.value())
        apply_event(event)
        print(
            f"partition={message.partition()} offset={message.offset()} "
            f"aggregate={event['aggregate_id']} type={event['type']}"
        )
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
