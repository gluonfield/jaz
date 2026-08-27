# Asynchronous workflows

Use the [shared setup](../README.md#setup) and run commands from `learnings/kafka`.

## Kafka feature

A request starts a workflow. A worker transactionally publishes completion while committing the request offset, and observers read committed results.

## Point

Durable commands and events let workflow stages run independently and resume after failures. Workflow and request IDs correlate every stage.

## System-design interview

Use this pattern to discuss orchestration versus choreography, retries, idempotency, correlation IDs, timeouts, compensation, and partial failure.

## Run

```sh
python asynchronous-workflows/observe.py
python asynchronous-workflows/worker.py
python asynchronous-workflows/start.py
```

Kafka transactions cover Kafka records and offsets. External side effects must still be idempotent.
