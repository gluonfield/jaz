# Proximity search

- **Feature:** `GEOADD` and `GEOSEARCH` index longitude and latitude.
- **Point:** Find nearby drivers, stores, or devices without scanning every location.
- **Interview:** Redis is strong for simple radius search; complex geographic queries need a spatial database.

```sh
uv run app.py
```
