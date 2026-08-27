# Session store

- **Feature:** A transaction creates the hash and its TTL atomically.
- **Point:** Every application server can read the same short-lived session.
- **Interview:** Fast logout and expiry are useful; durable user data remains in the primary database.

```sh
uv run app.py
```
