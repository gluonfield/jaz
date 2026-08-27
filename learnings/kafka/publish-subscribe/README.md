# Publish-subscribe

Use the [shared setup](../README.md#setup) and run commands from `learnings/kafka`.

## Kafka feature

Every consumer group maintains independent offsets and receives the topic's events. Consumers within one group divide its partitions.

## Point

One published event can independently drive email, analytics, billing, and other subscribers without coupling the producer to them.

## System-design interview

Use this pattern to explain fan-out, service independence, consumer lag, adding subscribers without changing producers, and the difference between a subscriber and its replicas.

## Run

```sh
python publish-subscribe/subscribe.py email-service
python publish-subscribe/subscribe.py analytics-service
python publish-subscribe/publish.py
```
