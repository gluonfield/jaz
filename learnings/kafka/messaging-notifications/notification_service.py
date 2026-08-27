import json
import os

from confluent_kafka import Consumer, KafkaException

TOPIC = "learning.messaging.messages"


def notify_connected_users(message):
    pass


consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": "notification-service",
        "auto.offset.reset": "earliest",
        "enable.auto.commit": False,
    }
)
consumer.subscribe([TOPIC])

try:
    while True:
        event = consumer.poll(1.0)
        if event is None:
            continue
        if event.error():
            raise KafkaException(event.error())
        message = json.loads(event.value())
        notify_connected_users(message)
        consumer.commit(message=event, asynchronous=False)
        print(
            f"notified for conversation={message['conversation_id']} "
            f"message={message['message_id']}"
        )
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
