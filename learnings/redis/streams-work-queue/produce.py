import os

from redis import Redis

STREAM = "jobs"
client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)
message_id = client.xadd(
    STREAM,
    {"kind": "send-email", "recipient": "user@example.com"},
)
print(f"queued {message_id}")
