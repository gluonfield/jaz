# Retry topics

## Kafka feature

On failure, a transaction writes the record to the next retry topic and commits the source offset atomically. After two retry topics, another failure routes the record to a DLQ.

## Point

The original partition can continue, but the retried record may now finish after later records. Transactions prevent a crash from committing the source without publishing its replacement, or publishing twice to Kafka.

## System-design interview

Choose this for throughput and bounded retries when strict original order is unnecessary. Discuss separate retry consumers or a scheduler for real delays, DLQ monitoring, idempotent non-Kafka side effects, and a distinct stable `transactional.id` for each live worker.

## Run

After the [shared setup](../../README.md#setup), run from `learnings/kafka`:

```sh
python consumer-retries/produce.py
python consumer-retries/retry-topics/consumer.py
```

The default record moves through both retry topics and then succeeds. Set `FAILURES_BEFORE_SUCCESS=9` when producing to see the DLQ path.
