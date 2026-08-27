# Consumer retry strategies

A classic Kafka consumer has a local read position and a durable committed offset. Delaying `commit()` makes a failed record eligible for redelivery after recovery; the application still has to implement retry timing, attempt limits, and DLQ routing.

The producer creates one record that intentionally fails twice. Each example has a different consumer group, so the same record can demonstrate all four strategies:

- [Block and retry in place](block-in-place/README.md): preserve order and block the consumer thread.
- [Stop and restart](stop-restart/README.md): let a supervisor restart from the committed offset.
- [Pause and seek](pause-seek/README.md): keep polling while one partition waits for its retry.
- [Retry topics](retry-topics/README.md): continue the source partition and trade away original order.

## Kafka Share Groups: TL;DR

Kafka 4.2 made [Share Groups](https://kafka.apache.org/43/javadoc/org/apache/kafka/clients/consumer/KafkaShareConsumer.html) production-ready for queue-like consumption. A `ShareConsumer` acquires individual records and acknowledges each one as `ACCEPT` (complete), `RELEASE` (redeliver), or `REJECT` (do not redeliver). Kafka owns the record lock and delivery count, so transient redelivery no longer has to be built from partition offsets.

Share Groups favor independent work items, flexible consumer scaling, and broker-managed redelivery. Classic consumer groups remain the fit for ordered partition streams and offset-based replay. The [`confluent-kafka` ShareConsumer](https://docs.confluent.io/kafka-clients/python/current/overview.html#kafka-share-consumers) is currently documented as preview, so these four examples use the established classic consumer API.
