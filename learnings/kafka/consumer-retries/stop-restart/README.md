# Stop and restart

## Kafka feature

An exception ends the consumer before `commit()`. On restart, the group resumes from its last committed offset and delivers the failed record again.

## Point

Kafka stores the recovery checkpoint; a process supervisor supplies restart and backoff. Kafka does not store an attempt count or automatically move a poison record to a DLQ.

## System-design interview

This is the smallest at-least-once design. Mention that the handler must be idempotent because it may finish its side effect and crash before the commit reaches Kafka.

## Run

After the [shared setup](../../README.md#setup), run from `learnings/kafka`:

```sh
python consumer-retries/produce.py
FAIL_PROCESSING=1 python consumer-retries/stop-restart/consumer.py
python consumer-retries/stop-restart/consumer.py
```

The first consumer exits intentionally. The second invocation receives and commits the same record.
