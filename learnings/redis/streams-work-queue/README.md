# Streams work queue

- **Feature:** `XADD`, consumer groups, `XREADGROUP`, and `XACK`.
- **Point:** One worker in the group handles each job; unacknowledged jobs remain pending.
- **Interview:** Processing is at least once. Use `XAUTOCLAIM` to recover abandoned jobs and make handlers idempotent.

```sh
uv run produce.py
uv run worker.py
```
