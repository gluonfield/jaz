# Cache

- **Feature:** `GET`, `SET`, and TTL.
- **Point:** Cache-aside reads the database only on a miss.
- **Interview:** Discuss invalidation, stale data, eviction, cache stampedes, and hot keys.

```sh
uv run app.py
```

Only the first read calls the mock database.
