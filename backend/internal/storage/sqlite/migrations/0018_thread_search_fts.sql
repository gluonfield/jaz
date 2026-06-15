-- +goose Up
CREATE TABLE IF NOT EXISTS message_search_docs (
  id INTEGER PRIMARY KEY,
  thread_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  UNIQUE(thread_id, seq)
);

CREATE VIRTUAL TABLE IF NOT EXISTS message_search_fts USING fts5(
  content,
  content='message_search_docs',
  content_rowid='id',
  tokenize='unicode61',
  prefix='2 3 4'
);

INSERT INTO message_search_docs(thread_id, seq, role, content)
SELECT thread_id, seq, role, content
FROM messages
WHERE role IN ('user', 'assistant')
  AND trim(content) <> '';

INSERT INTO message_search_fts(rowid, content)
SELECT id, content
FROM message_search_docs;

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_fts_ai
AFTER INSERT ON messages
WHEN new.role IN ('user', 'assistant') AND trim(new.content) <> ''
BEGIN
  INSERT INTO message_search_docs(thread_id, seq, role, content)
  VALUES (new.thread_id, new.seq, new.role, new.content);

  INSERT INTO message_search_fts(rowid, content)
  SELECT id, content
  FROM message_search_docs
  WHERE thread_id = new.thread_id AND seq = new.seq;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_fts_ad
AFTER DELETE ON messages
BEGIN
  INSERT INTO message_search_fts(message_search_fts, rowid, content)
  SELECT 'delete', id, content
  FROM message_search_docs
  WHERE thread_id = old.thread_id AND seq = old.seq;

  DELETE FROM message_search_docs
  WHERE thread_id = old.thread_id AND seq = old.seq;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_fts_au
AFTER UPDATE ON messages
BEGIN
  INSERT INTO message_search_fts(message_search_fts, rowid, content)
  SELECT 'delete', id, content
  FROM message_search_docs
  WHERE thread_id = old.thread_id AND seq = old.seq;

  DELETE FROM message_search_docs
  WHERE thread_id = old.thread_id AND seq = old.seq;

  INSERT INTO message_search_docs(thread_id, seq, role, content)
  SELECT new.thread_id, new.seq, new.role, new.content
  WHERE new.role IN ('user', 'assistant') AND trim(new.content) <> '';

  INSERT INTO message_search_fts(rowid, content)
  SELECT id, content
  FROM message_search_docs
  WHERE thread_id = new.thread_id AND seq = new.seq;
END;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS thread_search_docs (
  id INTEGER PRIMARY KEY,
  thread_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  slug TEXT NOT NULL,
  project_path TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS thread_search_fts USING fts5(
  title,
  slug,
  project_path,
  content='thread_search_docs',
  content_rowid='id',
  tokenize='unicode61',
  prefix='2 3 4'
);

INSERT INTO thread_search_docs(thread_id, title, slug, project_path)
SELECT id, coalesce(title, ''), slug, coalesce(project_path, '')
FROM threads;

INSERT INTO thread_search_fts(rowid, title, slug, project_path)
SELECT id, title, slug, project_path
FROM thread_search_docs;

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS thread_search_fts_ai
AFTER INSERT ON threads
BEGIN
  INSERT INTO thread_search_docs(thread_id, title, slug, project_path)
  VALUES (new.id, coalesce(new.title, ''), new.slug, coalesce(new.project_path, ''));

  INSERT INTO thread_search_fts(rowid, title, slug, project_path)
  SELECT id, title, slug, project_path
  FROM thread_search_docs
  WHERE thread_id = new.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS thread_search_fts_ad
AFTER DELETE ON threads
BEGIN
  INSERT INTO thread_search_fts(thread_search_fts, rowid, title, slug, project_path)
  SELECT 'delete', id, title, slug, project_path
  FROM thread_search_docs
  WHERE thread_id = old.id;

  DELETE FROM thread_search_docs
  WHERE thread_id = old.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS thread_search_fts_au
AFTER UPDATE OF title, slug, project_path ON threads
BEGIN
  INSERT INTO thread_search_fts(thread_search_fts, rowid, title, slug, project_path)
  SELECT 'delete', id, title, slug, project_path
  FROM thread_search_docs
  WHERE thread_id = old.id;

  DELETE FROM thread_search_docs
  WHERE thread_id = old.id;

  INSERT INTO thread_search_docs(thread_id, title, slug, project_path)
  VALUES (new.id, coalesce(new.title, ''), new.slug, coalesce(new.project_path, ''));

  INSERT INTO thread_search_fts(rowid, title, slug, project_path)
  SELECT id, title, slug, project_path
  FROM thread_search_docs
  WHERE thread_id = new.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS thread_search_fts_au;
DROP TRIGGER IF EXISTS thread_search_fts_ad;
DROP TRIGGER IF EXISTS thread_search_fts_ai;
DROP TABLE IF EXISTS thread_search_fts;
DROP TABLE IF EXISTS thread_search_docs;

DROP TRIGGER IF EXISTS message_search_fts_au;
DROP TRIGGER IF EXISTS message_search_fts_ad;
DROP TRIGGER IF EXISTS message_search_fts_ai;
DROP TABLE IF EXISTS message_search_fts;
DROP TABLE IF EXISTS message_search_docs;
