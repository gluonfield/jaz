# Data pipelines and ETL

Use the [shared setup](../README.md#setup) and run commands from `learnings/kafka`.

## Kafka feature

Kafka durably buffers records between an external source and sink. The sink manually commits after completing its external write.

## Point

Sources and destinations can run at different speeds or recover independently. The external boundary provides at-least-once delivery, so writes must be idempotent.

## System-design interview

Use this pattern to discuss backpressure, burst absorption, retries, deduplication keys, consumer lag, and why Kafka transactions cannot atomically cover an arbitrary database.

## Run

```sh
python data-pipelines-etl/sink.py
python data-pipelines-etl/source.py
```
