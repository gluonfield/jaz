# Distributed lock

- **Feature:** `SET NX EX` acquires; token-checked Lua releases.
- **Point:** Expiry prevents a crashed client from holding the lock forever.
- **Interview:** Treat Redis locks as coordination, not the final correctness boundary; stale owners and failover can violate exclusivity.

```sh
uv run app.py
```
