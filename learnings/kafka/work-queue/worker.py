import json
import os

from confluent_kafka import Consumer, KafkaException

TOPIC = "learning.work-queue.tasks"


def perform_task(task):
    pass


consumer = Consumer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "group.id": "task-workers",
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
        task = json.loads(message.value())
        perform_task(task)
        consumer.commit(message=message, asynchronous=False)
        print(f"completed {task['task_id']}")
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
