import json
import os

from confluent_kafka import Consumer, KafkaException

TOPIC = "learning.event-bus.orders"


def handle_order_event(event):
    pass


consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": "inventory-service",
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
        handle_order_event(event)
        consumer.commit(message=message, asynchronous=False)
        print(f"handled {event['type']} for {event['aggregate_id']}")
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
