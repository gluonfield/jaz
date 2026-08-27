import json
import os

from confluent_kafka import Consumer, KafkaException, Producer, TopicPartition

INPUT_TOPIC = "learning.consumer-retries.input"
RETRY_TOPICS = [
    "learning.consumer-retries.retry-1",
    "learning.consumer-retries.retry-2",
]
DLQ_TOPIC = "learning.consumer-retries.dlq"


class TransientError(Exception):
    pass


def handle_event(event):
    if event["attempt"] < event["failures_before_success"]:
        raise TransientError("mock transient failure")


bootstrap_servers = os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
consumer = Consumer(
    {
        "bootstrap.servers": bootstrap_servers,
        "group.id": "retry-topic-workers",
        "auto.offset.reset": "earliest",
        "enable.auto.commit": False,
        "isolation.level": "read_committed",
    }
)
producer = Producer(
    {
        "bootstrap.servers": bootstrap_servers,
        "transactional.id": os.getenv("KAFKA_TRANSACTIONAL_ID", "retry-topic-worker-1"),
        "enable.idempotence": True,
    }
)
producer.init_transactions()
consumer.subscribe([INPUT_TOPIC, *RETRY_TOPICS])

try:
    while True:
        message = consumer.poll(1)
        if message is None:
            continue
        if message.error():
            raise KafkaException(message.error())

        event = json.loads(message.value())
        producer.begin_transaction()
        try:
            try:
                handle_event(event)
            except TransientError:
                attempt = event["attempt"]
                target = (
                    RETRY_TOPICS[attempt] if attempt < len(RETRY_TOPICS) else DLQ_TOPIC
                )
                event["attempt"] = attempt + 1
                producer.produce(
                    target,
                    key=message.key(),
                    value=json.dumps(event),
                )
                outcome = f"routed offset {message.offset()} to {target}"
            else:
                outcome = (
                    f"handled {event['event_id']} on attempt {event['attempt'] + 1}"
                )

            offsets = [
                TopicPartition(
                    message.topic(),
                    message.partition(),
                    message.offset() + 1,
                )
            ]
            producer.send_offsets_to_transaction(
                offsets,
                consumer.consumer_group_metadata(),
            )
            producer.commit_transaction()
            print(outcome)
        except BaseException:
            producer.abort_transaction()
            raise
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
