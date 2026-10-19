-- +goose Up
-- A thread that one of a bot's subtasks, group seats or routine runs starts is
-- the bot's work too, under the thread that started it. Those started before
-- now carry no source, so they and their usage read as the user's own chats.
WITH RECURSIVE work(id, bot) AS (
  SELECT id, source_id FROM threads WHERE source_type IN ('bot_worker', 'bot_member')
  UNION
  SELECT id, parent_id FROM threads WHERE source_type = 'loop_run' AND COALESCE(parent_id, '') <> ''
  UNION
  SELECT t.id, work.bot FROM threads t JOIN work ON t.parent_id = work.id
  WHERE COALESCE(t.source_type, '') = ''
)
UPDATE threads
SET source_type = 'bot_worker', source_id = (SELECT bot FROM work WHERE work.id = threads.id)
WHERE COALESCE(source_type, '') = '' AND id IN (SELECT id FROM work);

UPDATE usage_events SET source_type = 'bot_worker'
WHERE source_type = '' AND thread_id IN (SELECT id FROM threads WHERE source_type = 'bot_worker');

-- A thread's own children now include the bot's work it started.
DROP INDEX idx_threads_overview_children;
CREATE INDEX idx_threads_overview_children
ON threads(parent_id, last_attention_at_ms DESC, id)
WHERE runtime = 'acp' AND (COALESCE(source_type, '') = '' OR (source_type = 'bot_worker' AND source_id <> parent_id));

-- +goose Down
DROP INDEX idx_threads_overview_children;
CREATE INDEX idx_threads_overview_children
ON threads(parent_id, last_attention_at_ms DESC, id)
WHERE runtime = 'acp' AND COALESCE(source_type, '') = '';

UPDATE usage_events SET source_type = ''
WHERE thread_id IN (SELECT id FROM threads WHERE source_type = 'bot_worker' AND parent_id <> source_id);

UPDATE threads SET source_type = NULL, source_id = NULL
WHERE source_type = 'bot_worker' AND parent_id <> source_id;
