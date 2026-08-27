# Pub/Sub

- **Feature:** `PUBLISH` broadcasts to every connected subscriber.
- **Point:** Simple low-latency fan-out for live chat, presence, and notifications.
- **Interview:** Delivery is at most once; disconnected subscribers miss messages. Use Streams or Kafka when consumers must catch up.

Start the subscriber first:

```sh
uv run subscribe.py
uv run publish.py
```
