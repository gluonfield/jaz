# Redis patterns in Python

Minimal [`redis-py`](https://redis.readthedocs.io/) examples for common system-design patterns.

## Setup

Run from this directory:

```sh
uv sync
docker compose up -d
```

`uv run` finds this project from every example directory. Use `REDIS_URL` to override `redis://localhost:6379`.

## Examples

- [Cache](cache/README.md)
- [Session store](session-store/README.md)
- [Rate limiter](rate-limiter/README.md)
- [Distributed lock](distributed-lock/README.md)
- [Leaderboard](leaderboard/README.md)
- [Proximity search](proximity-search/README.md)
- [Streams work queue](streams-work-queue/README.md)
- [Pub/Sub](pub-sub/README.md)

## Redis versus Kafka

| Pattern | Redis | Kafka |
|---|---|---|
| Live fan-out | Pub/Sub; offline subscribers miss messages | Retained records let consumer groups catch up |
| Work queue | Streams consumer groups | Classic consumer groups or Share Groups |
| Append-only events | Streams | Partitioned topics with longer retention and replay |

Redis can replace Kafka for modest queues, short-lived streams, and live fan-out when Redis is already deployed. Kafka is the fit for durable event history, long retention, large ordered pipelines, and many independent consumers.
