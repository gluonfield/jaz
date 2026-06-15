-- +goose Up
CREATE TABLE IF NOT EXISTS message_search_docs (
  id INTEGER PRIMARY KEY,
  thread_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL,
  UNIQUE(thread_id, seq),
  FOREIGN KEY (thread_id, seq) REFERENCES messages(thread_id, seq) ON DELETE CASCADE
);

CREATE VIEW IF NOT EXISTS message_search_content AS
SELECT d.id, d.thread_id, d.seq, d.role, m.content
FROM message_search_docs d
JOIN messages m ON m.thread_id = d.thread_id AND m.seq = d.seq;

CREATE VIRTUAL TABLE IF NOT EXISTS message_search_fts USING fts5(
  content,
  content='message_search_content',
  content_rowid='id',
  tokenize='unicode61',
  prefix='2 3 4'
);

INSERT INTO message_search_docs(thread_id, seq, role)
SELECT thread_id, seq, role
FROM messages
WHERE role IN ('user', 'assistant')
  AND trim(content) <> '';

INSERT INTO message_search_fts(rowid, content)
SELECT id, content
FROM message_search_content;

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_fts_ai
AFTER INSERT ON messages
WHEN new.role IN ('user', 'assistant') AND trim(new.content) <> ''
BEGIN
  INSERT INTO message_search_docs(thread_id, seq, role)
  VALUES (new.thread_id, new.seq, new.role);

  INSERT INTO message_search_fts(rowid, content)
  SELECT id, content
  FROM message_search_content
  WHERE thread_id = new.thread_id AND seq = new.seq;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_fts_bd
BEFORE DELETE ON messages
BEGIN
  INSERT INTO message_search_fts(message_search_fts, rowid, content)
  SELECT 'delete', id, content
  FROM message_search_content
  WHERE thread_id = old.thread_id AND seq = old.seq;

  DELETE FROM message_search_docs
  WHERE thread_id = old.thread_id AND seq = old.seq;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_fts_bu
BEFORE UPDATE ON messages
BEGIN
  INSERT INTO message_search_fts(message_search_fts, rowid, content)
  SELECT 'delete', id, content
  FROM message_search_content
  WHERE thread_id = old.thread_id AND seq = old.seq;

  DELETE FROM message_search_docs
  WHERE thread_id = old.thread_id AND seq = old.seq;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_fts_au
AFTER UPDATE ON messages
BEGIN
  INSERT INTO message_search_docs(thread_id, seq, role)
  SELECT new.thread_id, new.seq, new.role
  WHERE new.role IN ('user', 'assistant') AND trim(new.content) <> '';

  INSERT INTO message_search_fts(rowid, content)
  SELECT id, content
  FROM message_search_content
  WHERE thread_id = new.thread_id AND seq = new.seq;
END;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS thread_search_docs (
  id INTEGER PRIMARY KEY,
  thread_id TEXT NOT NULL UNIQUE,
  FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
);

CREATE VIEW IF NOT EXISTS thread_search_content AS
SELECT
  d.id,
  d.thread_id,
  coalesce(t.title, '') AS title,
  t.slug,
  coalesce(t.project_path, '') AS project_path
FROM thread_search_docs d
JOIN threads t ON t.id = d.thread_id;

CREATE VIRTUAL TABLE IF NOT EXISTS thread_search_fts USING fts5(
  title,
  slug,
  project_path,
  content='thread_search_content',
  content_rowid='id',
  tokenize='unicode61',
  prefix='2 3 4'
);

INSERT INTO thread_search_docs(thread_id)
SELECT id
FROM threads;

INSERT INTO thread_search_fts(rowid, title, slug, project_path)
SELECT id, title, slug, project_path
FROM thread_search_content;

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS thread_search_fts_ai
AFTER INSERT ON threads
BEGIN
  INSERT INTO thread_search_docs(thread_id)
  VALUES (new.id);

  INSERT INTO thread_search_fts(rowid, title, slug, project_path)
  SELECT id, title, slug, project_path
  FROM thread_search_content
  WHERE thread_id = new.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS thread_search_fts_bd
BEFORE DELETE ON threads
BEGIN
  INSERT INTO thread_search_fts(thread_search_fts, rowid, title, slug, project_path)
  SELECT 'delete', id, title, slug, project_path
  FROM thread_search_content
  WHERE thread_id = old.id;

  DELETE FROM thread_search_docs
  WHERE thread_id = old.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS thread_search_fts_bu
BEFORE UPDATE OF title, slug, project_path ON threads
BEGIN
  INSERT INTO thread_search_fts(thread_search_fts, rowid, title, slug, project_path)
  SELECT 'delete', id, title, slug, project_path
  FROM thread_search_content
  WHERE thread_id = old.id;

  DELETE FROM thread_search_docs
  WHERE thread_id = old.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS thread_search_fts_au
AFTER UPDATE OF title, slug, project_path ON threads
BEGIN
  INSERT INTO thread_search_docs(thread_id)
  VALUES (new.id);

  INSERT INTO thread_search_fts(rowid, title, slug, project_path)
  SELECT id, title, slug, project_path
  FROM thread_search_content
  WHERE thread_id = new.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS thread_search_fts_au;
DROP TRIGGER IF EXISTS thread_search_fts_bu;
DROP TRIGGER IF EXISTS thread_search_fts_bd;
DROP TRIGGER IF EXISTS thread_search_fts_ai;
DROP TABLE IF EXISTS thread_search_fts;
DROP VIEW IF EXISTS thread_search_content;
DROP TABLE IF EXISTS thread_search_docs;

DROP TRIGGER IF EXISTS message_search_fts_au;
DROP TRIGGER IF EXISTS message_search_fts_bu;
DROP TRIGGER IF EXISTS message_search_fts_bd;
DROP TRIGGER IF EXISTS message_search_fts_ai;
DROP TABLE IF EXISTS message_search_fts;
DROP VIEW IF EXISTS message_search_content;
DROP TABLE IF EXISTS message_search_docs;
