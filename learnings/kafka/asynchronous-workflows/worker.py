import json
import os
import uuid

from confluent_kafka import Consumer, KafkaException, Producer, TopicPartition

INPUT_TOPIC = "learning.workflow.requests"
OUTPUT_TOPIC = "learning.workflow.completed"
GROUP_ID = "workflow-workers"
bootstrap_servers = os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")


def execute_step(request):
    pass


consumer = Consumer(
    {
        "bootstrap.servers": bootstrap_servers,
        "group.id": GROUP_ID,
        "auto.offset.reset": "earliest",
        "enable.auto.commit": False,
        "isolation.level": "read_committed",
    }
)
producer = Producer(
    {
        "bootstrap.servers": bootstrap_servers,
        "transactional.id": os.getenv("KAFKA_TRANSACTIONAL_ID", "workflow-worker-1"),
        "enable.idempotence": True,
    }
)
producer.init_transactions()
consumer.subscribe([INPUT_TOPIC])

try:
    while True:
        message = consumer.poll(1.0)
        if message is None:
            continue
        if message.error():
            raise KafkaException(message.error())
        request = json.loads(message.value())
        producer.begin_transaction()
        try:
            execute_step(request)
            completed = {
                "workflow_id": request["workflow_id"],
                "request_id": request["request_id"],
                "event_id": str(uuid.uuid4()),
                "type": "example.completed",
                "data": {},
            }
            producer.produce(
                OUTPUT_TOPIC,
                key=message.key(),
                value=json.dumps(completed).encode(),
            )
            next_offset = TopicPartition(
                message.topic(),
                message.partition(),
                message.offset() + 1,
            )
            producer.send_offsets_to_transaction(
                [next_offset],
                consumer.consumer_group_metadata(),
            )
            producer.commit_transaction()
            print(f"completed {request['workflow_id']}")
        except BaseException:
            producer.abort_transaction()
            raise
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
