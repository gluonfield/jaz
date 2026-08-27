# Pause and seek

## Kafka feature

`pause()` stops delivery from the failed partition while polling continues. When the retry is due, `seek()` restores the failed offset and `resume()` makes that partition available again.

## Point

Other assigned partitions can keep moving while this partition waits, and the failed offset remains uncommitted. The application owns scheduling, backoff, attempt state, rebalance handling, and the exhausted-retry policy.

## System-design interview

Use this to explain that consumer position is local and movable while the committed offset is the durable recovery checkpoint. The example keeps attempts in memory; a restart-safe design persists them.

## Run

After the [shared setup](../../README.md#setup), run from `learnings/kafka`:

```sh
python consumer-retries/produce.py
python consumer-retries/pause-seek/consumer.py
```

The partition is paused for one second, then two seconds, before the third attempt succeeds.
