import os

from confluent_kafka import Consumer, KafkaException, Producer, TopicPartition

INPUT_TOPIC = "learning.stream.raw"
OUTPUT_TOPIC = "learning.stream.processed"
GROUP_ID = "stream-processors"
bootstrap_servers = os.getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")


def transform(value):
    return value


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
        "transactional.id": os.getenv("KAFKA_TRANSACTIONAL_ID", "stream-processor-1"),
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
        producer.begin_transaction()
        try:
            producer.produce(
                OUTPUT_TOPIC,
                key=message.key(),
                value=transform(message.value()),
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
            print(f"processed offset {message.offset()}")
        except BaseException:
            producer.abort_transaction()
            raise
except KeyboardInterrupt:
    pass
finally:
    consumer.close()
