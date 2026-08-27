import json
import os

from confluent_kafka import Consumer, KafkaException

TOPIC = "learning.workflow.completed"


def handle_completion(event):
    pass


consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": "workflow-observers",
        "auto.offset.reset": "earliest",
        "enable.auto.commit": False,
        "isolation.level": "read_committed",
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
        handle_completion(event)
        consumer.commit(message=message, asynchronous=False)
        print(f"observed {event['workflow_id']}")
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
