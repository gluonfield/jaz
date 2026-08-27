import os

from redis import Redis
from redis.exceptions import ResponseError

STREAM = "jobs"
GROUP = "workers"
CONSUMER = os.getenv("CONSUMER_NAME", "worker-1")

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)

try:
    client.xgroup_create(STREAM, GROUP, id="0", mkstream=True)
except ResponseError as error:
    if "BUSYGROUP" not in str(error):
        raise

streams = client.xreadgroup(
    GROUP,
    CONSUMER,
    {STREAM: ">"},
    count=1,
    block=5000,
)
if not streams:
    raise TimeoutError("no job arrived within five seconds")

_, messages = streams[0]
message_id, job = messages[0]
print(f"handled {job}")
client.xack(STREAM, GROUP, message_id)
print(f"acknowledged {message_id}")
