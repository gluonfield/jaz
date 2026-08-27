import json
import os
import sys

from confluent_kafka import Consumer, KafkaException

TOPIC = "learning.pub-sub.events"


def handle_event(event):
    pass


if len(sys.argv) != 2:
    raise SystemExit("usage: python subscribe.py <subscriber-name>")

subscriber_name = sys.argv[1]
consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": subscriber_name,
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
        event = json.loads(message.value())
        handle_event(event)
        consumer.commit(message=message, asynchronous=False)
        print(f"{subscriber_name} received {event['event_id']}")
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
