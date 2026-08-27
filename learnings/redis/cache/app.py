import json
import os

from redis import Redis

client = Redis.from_url(
    os.getenv("REDIS_URL", "redis://localhost:6379"),
    decode_responses=True,
)


def load_product(product_id):
    print("database read")
    return {"id": product_id, "name": "keyboard"}


def get_product(product_id):
    key = f"product:{product_id}"
    cached = client.get(key)
    if cached is not None:
        return json.loads(cached)

    product = load_product(product_id)
    client.set(key, json.dumps(product), ex=60)
    return product


client.delete("product:42")
print(get_product("42"))
print(get_product("42"))
