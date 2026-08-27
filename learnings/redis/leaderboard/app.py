import os

from redis import Redis

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)
key = "leaderboard:weekly"

client.delete(key)
client.zadd(key, {"alice": 120, "bob": 100, "carol": 140})
client.zincrby(key, 50, "bob")

for rank, (player, score) in enumerate(
    client.zrange(key, 0, -1, desc=True, withscores=True),
    start=1,
):
    print(rank, player, int(score))
