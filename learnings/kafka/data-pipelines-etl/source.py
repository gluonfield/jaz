import json
import os
import uuid

from confluent_kafka import KafkaException, Producer

TOPIC = "learning.etl.records"


def extract_record():
    return {"record_id": str(uuid.uuid4()), "data": {}}


def delivery_report(error, _message):
    if error is not None:
        raise KafkaException(error)


record = extract_record()
producer = Producer(
    {
        "bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"),
        "enable.idempotence": True,
    }
)
producer.produce(
    TOPIC,
    key=record["record_id"].encode(),
    value=json.dumps(record).encode(),
    on_delivery=delivery_report,
)

if producer.flush(10):
    raise TimeoutError("record was not delivered within 10 seconds")

print(record["record_id"])
