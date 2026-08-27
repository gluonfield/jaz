import os
import uuid

from redis import Redis

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)
session_id = str(uuid.uuid4())
key = f"session:{session_id}"

with client.pipeline(transaction=True) as pipeline:
    pipeline.hset(key, mapping={"user_id": "42", "role": "admin"})
    pipeline.expire(key, 1800)
    pipeline.execute()

print(client.hgetall(key))
print(f"expires in {client.ttl(key)} seconds")
client.delete(key)
