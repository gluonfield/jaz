-- +goose Up
WITH affected AS (
  SELECT DISTINCT thread_id
  FROM session_events
  WHERE type IN ('acp_message', 'acp_thought')
    AND projection_key = ''
    AND json_valid(acp)
    AND json_type(acp) = 'object'
    AND COALESCE(json_extract(acp, '$.id'), '') <> ''
    AND COALESCE(json_extract(acp, '$.text_run_id'), '') = ''
    AND json_extract(acp, '$.state') = 'idle'
),
ordered AS (
  SELECT
    thread_id,
    seq,
    type,
    CASE
      WHEN type IN ('acp_message', 'acp_thought')
        AND projection_key = ''
        AND json_valid(acp)
        AND json_type(acp) = 'object'
        AND COALESCE(json_extract(acp, '$.id'), '') <> ''
        AND COALESCE(json_extract(acp, '$.text_run_id'), '') = ''
        AND json_extract(acp, '$.state') = 'idle'
      THEN 1 ELSE 0
    END AS broken,
    CASE
      WHEN json_valid(acp) AND json_type(acp) = 'object'
      THEN COALESCE(json_extract(acp, '$.id'), '')
      ELSE ''
    END AS acp_id
  FROM session_events
  WHERE thread_id IN (SELECT thread_id FROM affected)
    AND type NOT IN ('agent_session', 'agent_task')
),
flagged AS (
  SELECT
    thread_id,
    seq,
    broken,
    CASE
      WHEN broken = 1
        AND LAG(broken) OVER (PARTITION BY thread_id ORDER BY seq) = 1
        AND LAG(type) OVER (PARTITION BY thread_id ORDER BY seq) = type
        AND LAG(acp_id) OVER (PARTITION BY thread_id ORDER BY seq) = acp_id
      THEN 0 ELSE 1
    END AS new_group
  FROM ordered
),
grouped AS (
  SELECT
    thread_id,
    seq,
    broken,
    SUM(new_group) OVER (PARTITION BY thread_id ORDER BY seq) AS grp
  FROM flagged
),
runs AS (
  SELECT
    thread_id,
    seq,
    'post:' || MIN(seq) OVER (PARTITION BY thread_id, grp) AS run_id
  FROM grouped
  WHERE broken = 1
)
UPDATE session_events
SET acp = json_set(acp, '$.text_run_id', runs.run_id)
FROM runs
WHERE session_events.thread_id = runs.thread_id
  AND session_events.seq = runs.seq;

UPDATE threads
SET event_compaction_version = 0
WHERE event_compaction_version = 1
  AND id IN (
    SELECT DISTINCT thread_id
    FROM session_events
    WHERE type IN ('acp_message', 'acp_thought')
      AND projection_key = ''
      AND json_valid(acp)
      AND json_type(acp) = 'object'
      AND json_extract(acp, '$.text_run_id') LIKE 'post:%'
  );

-- +goose Down
SELECT 1;
