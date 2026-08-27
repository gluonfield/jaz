import json
import os
import time
import uuid

from confluent_kafka import KafkaException, Producer

TOPIC = "learning.messaging.messages"


def delivery_report(error, _message):
    if error is not None:
        raise KafkaException(error)


conversation_id = os.getenv("CONVERSATION_ID", "conversation-1")
message = {
    "message_id": str(uuid.uuid4()),
    "conversation_id": conversation_id,
    "sent_at_ms": int(time.time() * 1000),
    "body": "example",
}
producer = Producer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "enable.idempotence": True,
    }
)
producer.produce(
    TOPIC,
    key=conversation_id.encode(),
    value=json.dumps(message).encode(),
    on_delivery=delivery_report,
)

if producer.flush(10):
    raise TimeoutError("message was not delivered within 10 seconds")

print(message["message_id"])
