# Rate limiter

- **Feature:** Atomic `INCR` and `EXPIRE` in Lua.
- **Point:** All servers share one fixed-window request counter.
- **Interview:** Lua prevents a crash between increment and expiry; discuss fixed versus sliding windows.

```sh
uv run app.py
```

Run it repeatedly: requests after the third are rejected until the 10-second window expires.
