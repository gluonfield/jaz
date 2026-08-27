import os

from redis import Redis

WINDOW_SECONDS = 10
LIMIT = 3
SCRIPT = """
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
"""

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)
count = client.eval(SCRIPT, 1, "rate:user:42", WINDOW_SECONDS)
print(f"request {count}: {'allowed' if count <= LIMIT else 'rejected'}")
