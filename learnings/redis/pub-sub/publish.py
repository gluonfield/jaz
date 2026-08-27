import os

from redis import Redis

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)
subscribers = client.publish("chat:room:42", "hello")
print(f"delivered to {subscribers} subscribers")
