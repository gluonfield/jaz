# Messaging and notifications

Use the [shared setup](../README.md#setup) and run commands from `learnings/kafka`.

## Kafka feature

The conversation ID is the record key, preserving message order within each conversation. A consumer group scales the notification gateways.

## Point

Kafka provides the durable backend message stream; gateways fan messages out to connected clients through WebSockets or push services.

## System-design interview

Use this pattern to separate durable ingestion from online delivery and discuss per-conversation ordering, offline users, hot conversations, fan-out, and delivery acknowledgements.

## Run

```sh
python messaging-notifications/notification_service.py
python messaging-notifications/send_message.py
```
