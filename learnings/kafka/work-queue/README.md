# Work queue

Use the [shared setup](../README.md#setup) and run commands from `learnings/kafka`.

## Kafka feature

Consumers sharing one `group.id` divide the topic's partitions. Manual commits record a task only after successful processing.

## Point

This turns a durable topic into horizontally distributed work with retry after worker failure. Partition count bounds the number of workers that can process concurrently.

## System-design interview

Use this pattern to discuss worker scaling, backpressure, partition assignment, at-least-once delivery, and why task handlers must tolerate duplicates.

## Run

```sh
python work-queue/worker.py
python work-queue/worker.py
python work-queue/enqueue.py
```
