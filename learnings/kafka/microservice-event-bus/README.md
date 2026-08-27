# Microservice event bus

Use the [shared setup](../README.md#setup) and run commands from `learnings/kafka`.

## Kafka feature

The producer publishes a domain-event envelope. The aggregate ID is the key, keeping events for one order in one partition and in order.

## Point

Services communicate through durable facts instead of synchronous calls. Each interested service uses its own stable consumer group.

## System-design interview

Use this pattern to discuss service decoupling, event contracts, per-entity ordering, eventual consistency, schema evolution, and failure isolation.

## Run

```sh
python microservice-event-bus/inventory_service.py
python microservice-event-bus/order_service.py
```
