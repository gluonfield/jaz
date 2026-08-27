import json
import os
import uuid

from confluent_kafka import KafkaException, Producer

TOPIC = "learning.workflow.requests"


def delivery_report(error, _message):
    if error is not None:
        raise KafkaException(error)


workflow_id = str(uuid.uuid4())
request = {
    "workflow_id": workflow_id,
    "request_id": str(uuid.uuid4()),
    "type": "example.requested",
    "data": {},
}
producer = Producer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "enable.idempotence": True,
    }
)
producer.produce(
    TOPIC,
    key=workflow_id.encode(),
    value=json.dumps(request).encode(),
    on_delivery=delivery_report,
)

if producer.flush(10):
    raise TimeoutError("workflow request was not delivered within 10 seconds")

print(workflow_id)
