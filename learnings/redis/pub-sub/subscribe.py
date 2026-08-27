import os

from redis import Redis

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)

with client.pubsub() as pubsub:
    pubsub.subscribe("chat:room:42")
    for message in pubsub.listen():
        if message["type"] == "message":
            print(message["data"])
            break
