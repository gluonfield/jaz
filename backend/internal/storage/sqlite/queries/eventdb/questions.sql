-- name: HasOpenQuestion :one
WITH permissions AS (
  SELECT seq, type, permission FROM session_events
  WHERE thread_id = sqlc.arg(thread_id)
    AND type IN ('permission_request', 'permission_response')
)
SELECT EXISTS (
  SELECT 1 FROM permissions AS request
  WHERE request.type = 'permission_request'
    AND json_array_length(request.permission, '$.questions') > 0
    AND COALESCE(json_extract(request.permission, '$.status'), '') NOT IN ('selected', 'cancelled')
    AND NOT EXISTS (
      SELECT 1 FROM permissions AS response
      WHERE response.type = 'permission_response'
        AND response.seq > request.seq
        AND json_extract(response.permission, '$.id') = json_extract(request.permission, '$.id')
        AND json_extract(response.permission, '$.status') IN ('selected', 'cancelled')
    )
);

-- name: ListSessionQuestionEvents :many
SELECT seq, type, permission
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
  AND type IN ('permission_request', 'permission_response')
  AND permission IS NOT NULL
ORDER BY seq;
