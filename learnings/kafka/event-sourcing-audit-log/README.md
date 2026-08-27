# Event sourcing and audit log

Use the [shared setup](../README.md#setup) and run commands from `learnings/kafka`.

## Kafka feature

Kafka retains an ordered log. Events use the aggregate ID as their key, and replay assigns every partition at `OFFSET_BEGINNING`.

## Point

Retained history can rebuild application state, create new projections, or provide an audit trail. Ordering is per aggregate rather than global.

## System-design interview

Use this pattern to discuss replay, recovery, auditability, retention cost, snapshots, schema evolution, and rebuilding state without stopping writers.

## Run

```sh
python event-sourcing-audit-log/replay.py
python event-sourcing-audit-log/append_events.py
```
