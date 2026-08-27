import json
import os
import uuid

from confluent_kafka import KafkaException, Producer

TOPIC = "learning.work-queue.tasks"


def delivery_report(error, _message):
    if error is not None:
        raise KafkaException(error)


task = {"task_id": str(uuid.uuid4()), "payload": {}}
producer = Producer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "enable.idempotence": True,
    }
)
producer.produce(
    TOPIC,
    key=task["task_id"].encode(),
    value=json.dumps(task).encode(),
    on_delivery=delivery_report,
)

if producer.flush(10):
    raise TimeoutError("task was not delivered within 10 seconds")

print(task["task_id"])
