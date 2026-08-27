import os
import uuid

from redis import Redis

LOCK_KEY = "lock:concert:343"
RELEASE_SCRIPT = """
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
end
return 0
"""

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)
token = str(uuid.uuid4())

if not client.set(LOCK_KEY, token, nx=True, ex=30):
    raise RuntimeError("lock is already held")

try:
    print("exclusive work")
finally:
    released = client.eval(RELEASE_SCRIPT, 1, LOCK_KEY, token)
    print(f"released: {bool(released)}")
