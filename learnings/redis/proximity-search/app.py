import os

from redis import Redis

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)
key = "drivers:london"

client.delete(key)
client.geoadd(
    key,
    [
        -0.1276,
        51.5072,
        "driver:1",
        -0.1425,
        51.5010,
        "driver:2",
        -0.2000,
        51.5000,
        "driver:3",
    ],
)
nearby = client.geosearch(
    key,
    longitude=-0.1276,
    latitude=51.5072,
    radius=3,
    unit="km",
    withdist=True,
    sort="ASC",
)

for driver, distance in nearby:
    print(driver, f"{distance:.2f} km")
