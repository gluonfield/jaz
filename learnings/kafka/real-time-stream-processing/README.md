# Real-time stream processing

Use the [shared setup](../README.md#setup) and run commands from `learnings/kafka`.

## Kafka feature

A consumer continuously transforms input into another topic. A Kafka transaction commits the output record and consumed offset atomically.

## Point

The processor can restart without exposing duplicate committed outputs between Kafka topics. Downstream consumers use `read_committed`.

## System-design interview

Use this pattern to discuss streaming versus batch processing, partition-level parallelism, consumer lag, stateful processing, and the scope of exactly-once guarantees.

## Run

```sh
python real-time-stream-processing/processor.py
python real-time-stream-processing/produce.py
```

Each concurrent processor needs a unique `KAFKA_TRANSACTIONAL_ID`.
