# Kafka patterns in Python

Minimal examples using [`confluent-kafka`](https://github.com/confluentinc/confluent-kafka-python). Business handlers do nothing so the Kafka SDK usage remains visible.

## Setup

Run commands from this directory:

```sh
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
docker compose up -d
python create_topics.py
```

Use `KAFKA_BOOTSTRAP_SERVERS` to override `localhost:9092`.

## Examples

- [Work queue](work-queue/README.md)
- [Publish-subscribe](publish-subscribe/README.md)
- [Microservice event bus](microservice-event-bus/README.md)
- [Event sourcing and audit log](event-sourcing-audit-log/README.md)
- [Real-time stream processing](real-time-stream-processing/README.md)
- [Data pipelines and ETL](data-pipelines-etl/README.md)
- [Messaging and notifications](messaging-notifications/README.md)
- [Asynchronous workflows](asynchronous-workflows/README.md)
- [Consumer retry strategies](consumer-retries/README.md)

JSON is used for readability. Production systems also need schemas, authentication, observability, retry policies, and capacity-aware retention.
