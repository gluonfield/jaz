import os

from confluent_kafka import KafkaError, KafkaException
from confluent_kafka.admin import AdminClient, NewTopic

TOPICS = [
    "learning.work-queue.tasks",
    "learning.pub-sub.events",
    "learning.event-bus.orders",
    "learning.event-sourcing.accounts",
    "learning.stream.raw",
    "learning.stream.processed",
    "learning.etl.records",
    "learning.messaging.messages",
    "learning.workflow.requests",
    "learning.workflow.completed",
    "learning.consumer-retries.input",
    "learning.consumer-retries.retry-1",
    "learning.consumer-retries.retry-2",
    "learning.consumer-retries.dlq",
]

admin = AdminClient(
    {"bootstrap.servers": os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")}
)
results = admin.create_topics(
    [NewTopic(topic, num_partitions=3, replication_factor=1) for topic in TOPICS]
)

for topic, result in results.items():
    try:
        result.result()
        print(f"created {topic}")
    except KafkaException as error:
        if error.args[0].code() != KafkaError.TOPIC_ALREADY_EXISTS:
            raise
        print(f"already exists: {topic}")
