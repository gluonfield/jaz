# Leaderboard

- **Feature:** Sorted sets rank members by score.
- **Point:** `ZADD`, `ZINCRBY`, and ranked reads stay fast as the board grows.
- **Interview:** Useful for games, trending items, and priority queues; decide how much history to retain.

```sh
uv run app.py
```
