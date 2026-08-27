# Block and retry in place

## Kafka feature

The consumer keeps retrying one delivered record and commits its offset only after the handler succeeds.

## Point

This preserves processing order because later records cannot pass the failed record. The consumer thread stops polling while it retries, so every partition assigned to that thread waits. Long retries must stay within `max.poll.interval.ms` or move heartbeats/retries to a different design.

## System-design interview

Choose this when order matters more than throughput and failures are brief. State the poison-record policy: after the bounded attempts here, the process exits with the offset uncommitted.

## Run

After the [shared setup](../../README.md#setup), run from `learnings/kafka`:

```sh
python consumer-retries/produce.py
python consumer-retries/block-in-place/consumer.py
```

The mock handler fails twice, succeeds on attempt three, and then commits.
