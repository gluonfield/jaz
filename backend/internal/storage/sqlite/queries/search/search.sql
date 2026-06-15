-- name: SearchThreadMessages :many
SELECT
  t.id,
  t.slug,
  coalesce(t.title, '') AS title,
  t.status,
  t.runtime,
  coalesce(t.parent_id, '') AS parent_id,
  t.archived,
  d.seq,
  snippet(message_search_fts, 0, char(31), char(30), '...', 18) AS snippet,
  bm25(message_search_fts) AS score,
  t.updated_at_ms,
  t.last_attention_at_ms
FROM message_search_fts(CAST(sqlc.arg(match) AS TEXT))
JOIN message_search_docs d ON d.id = message_search_fts.rowid
JOIN threads t ON t.id = d.thread_id
WHERE (CAST(sqlc.arg(include_archived) AS INTEGER) = 1 OR t.archived = 0)
ORDER BY bm25(message_search_fts)
LIMIT sqlc.arg(limit);

-- name: SearchThreadMetadata :many
SELECT
  t.id,
  t.slug,
  coalesce(t.title, '') AS title,
  t.status,
  t.runtime,
  coalesce(t.parent_id, '') AS parent_id,
  t.archived,
  snippet(thread_search_fts, 0, char(31), char(30), '...', 12) AS title_snippet,
  snippet(thread_search_fts, 1, char(31), char(30), '...', 12) AS slug_snippet,
  bm25(thread_search_fts, 3.0, 2.0, 1.0) AS score,
  t.updated_at_ms,
  t.last_attention_at_ms
FROM thread_search_fts(CAST(sqlc.arg(match) AS TEXT))
JOIN thread_search_docs d ON d.id = thread_search_fts.rowid
JOIN threads t ON t.id = d.thread_id
WHERE (CAST(sqlc.arg(include_archived) AS INTEGER) = 1 OR t.archived = 0)
ORDER BY bm25(thread_search_fts, 3.0, 2.0, 1.0)
LIMIT sqlc.arg(limit);
