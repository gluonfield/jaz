package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	stdjson "encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	_ "modernc.org/sqlite"
)

type Store struct {
	root   string
	db     *sql.DB
	mirror *jsonstore.Store
	mu     sync.Mutex
}

func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".jaz"
	}
	return filepath.Join(home, ".jaz")
}

func New(root string) (*Store, error) {
	if root == "" {
		root = DefaultRoot()
	}
	store := &Store{root: root}
	for _, dir := range []string{store.RootDir(), store.SessionsDir(), store.WorkspacesDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	mirror, err := jsonstore.New(root)
	if err != nil {
		return nil, err
	}
	store.mirror = mirror
	db, err := sql.Open("sqlite", filepath.Join(root, "jaz.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store.db = db
	if err := store.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.importLegacyJSON(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.backfillMissingThreadErrors(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.resetStaleRunningThreads(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) RootDir() string {
	return s.root
}

func (s *Store) SessionsDir() string {
	return filepath.Join(s.root, "sessions")
}

func (s *Store) WorkspacesDir() string {
	return filepath.Join(s.root, "workspaces")
}

func (s *Store) DefaultWorkspace() string {
	return filepath.Join(s.WorkspacesDir(), "default")
}

func (s *Store) NewSessionID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-00000000", time.Now().UTC().Format("20060102T150405"))
	}
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405"), hex.EncodeToString(b[:]))
}

func (s *Store) CreateSession(input storage.CreateSession) (storage.Session, error) {
	s.mu.Lock()

	now := time.Now().UTC()
	session := storage.Session{
		ID:         s.NewSessionID(),
		Slug:       input.Slug,
		Title:      input.Title,
		ParentID:   input.ParentID,
		Status:     storage.StatusIdle,
		Runtime:    firstNonEmpty(input.Runtime, storage.RuntimeNative),
		RuntimeRef: input.RuntimeRef,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if session.Slug == "" {
		session.Slug = defaultSlug(session)
	}
	slug, err := s.uniqueSlugLocked(session.Slug, "")
	if err != nil {
		s.mu.Unlock()
		return storage.Session{}, err
	}
	session.Slug = slug
	if err := s.saveSessionLocked(session, false); err != nil {
		s.mu.Unlock()
		return storage.Session{}, err
	}
	s.mu.Unlock()
	s.mirrorSession(session)
	return session, nil
}

func (s *Store) EnsureSession(id string) error {
	if id == "" {
		return fmt.Errorf("session id is empty")
	}
	return nil
}

func (s *Store) LoadSession(ref string) (storage.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSessionLocked(ref)
}

func (s *Store) SaveSession(session storage.Session) error {
	s.mu.Lock()
	err := s.saveSessionLocked(session, true)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if current, err := s.LoadSession(session.ID); err == nil {
		s.mirrorSession(current)
	}
	return nil
}

// SetArchived archives or restores a session together with its children.
func (s *Store) SetArchived(id string, archived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	flag := 0
	if archived {
		flag = 1
	}
	_, err := s.db.Exec(`UPDATE threads SET archived = ? WHERE id = ? OR parent_id = ?`, flag, id, id)
	return err
}

func (s *Store) ListSessions(filter storage.SessionFilter) ([]storage.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT id, slug, title, parent_id, status, error, runtime, acp_agent, acp_session_id, cwd,
model_provider, model, reasoning_effort, input_tokens, cached_input_tokens, output_tokens,
reasoning_output_tokens, total_tokens, archived, created_at_ms, updated_at_ms FROM threads`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []storage.Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		if filter.RootOnly && session.ParentID != "" {
			continue
		}
		if filter.ParentOnly && session.ParentID != filter.ParentID {
			continue
		}
		if !filter.IncludeChildren && !filter.ParentOnly && !filter.RootOnly && filter.ParentID == "" && session.ParentID != "" {
			continue
		}
		if filter.ParentID != "" && session.ParentID != filter.ParentID {
			continue
		}
		if filter.Runtime != "" && session.Runtime != filter.Runtime {
			continue
		}
		if session.Archived != filter.Archived {
			continue
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	if filter.Limit > 0 && len(sessions) > filter.Limit {
		sessions = sessions[:filter.Limit]
	}
	return sessions, nil
}

func (s *Store) LastRootSession() (storage.Session, error) {
	sessions, err := s.ListSessions(storage.SessionFilter{RootOnly: true, Limit: 1})
	if err != nil {
		return storage.Session{}, err
	}
	if len(sessions) == 0 {
		return storage.Session{}, fmt.Errorf("no root sessions found")
	}
	return sessions[0], nil
}

func (s *Store) LoadMessages(id string) ([]provider.Message, error) {
	records, err := s.LoadMessageRecords(id)
	if err != nil {
		return nil, err
	}
	return providerMessagesFromRecords(records)
}

func (s *Store) LoadMessageRecords(id string) ([]storage.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadMessageRecordsLocked(id)
}

func (s *Store) SaveMessages(id string, messages []provider.Message) error {
	records, err := recordsFromProviderMessages(messages, time.Now().UTC())
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.replaceMessagesLocked(id, records)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.mirrorMessages(id, messages)
	return nil
}

func (s *Store) SaveMessagesWithReasoning(id string, messages []provider.Message, reasoningByMessage map[int]string) error {
	records, err := recordsFromProviderMessagesWithReasoning(messages, reasoningByMessage, time.Now().UTC())
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.replaceMessagesLocked(id, records)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.mirrorMessages(id, messages)
	return nil
}

func (s *Store) AppendMessages(id string, messages ...provider.Message) error {
	if len(messages) == 0 {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	records, err := s.loadMessageRecordsLocked(id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	next, err := recordsFromProviderMessages(messages, now)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	for i := range next {
		next[i].ThreadID = id
		next[i].Seq = int64(len(records) + i + 1)
		next[i].CreatedAt = now.Add(time.Duration(i+1) * time.Millisecond)
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	defer tx.Rollback()
	for _, record := range next {
		if err := insertMessage(tx, record); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	_, err = tx.Exec(`UPDATE threads SET updated_at_ms = ? WHERE id = ?`, timeToMs(now), id)
	if err == nil {
		err = tx.Commit()
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if s.mirror != nil {
		_ = s.mirror.AppendMessages(id, messages...)
	}
	return nil
}

func (s *Store) LoadActivity(id string) ([]storage.ActivityEntry, error) {
	if s.mirror == nil {
		return nil, nil
	}
	return s.mirror.LoadActivity(id)
}

func (s *Store) SaveActivity(id string, activity []storage.ActivityEntry) error {
	if s.mirror == nil {
		return nil
	}
	return s.mirror.SaveActivity(id, activity)
}

func (s *Store) UpsertActivity(id string, entry storage.ActivityEntry) error {
	if s.mirror == nil {
		return nil
	}
	return s.mirror.UpsertActivity(id, entry)
}

func (s *Store) LoadACPState(id string) (storage.ACPState, error) {
	if s.mirror == nil {
		return storage.ACPState{}, fmt.Errorf("acp state store is not configured")
	}
	return s.mirror.LoadACPState(id)
}

func (s *Store) SaveACPState(id string, state storage.ACPState) error {
	if id == "" {
		return fmt.Errorf("session id is empty")
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if s.mirror == nil {
		return fmt.Errorf("acp state store is not configured")
	}
	if err := s.mirror.SaveACPState(id, state); err != nil {
		return err
	}

	status := storage.SessionStatusForACPState(state.State)
	errorMessage := ""
	if status == storage.StatusError {
		errorMessage = state.Error
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if status == "" {
		_, err := s.db.Exec(`UPDATE threads SET updated_at_ms = ? WHERE id = ?`, timeToMs(state.UpdatedAt), id)
		return err
	}
	_, err := s.db.Exec(`UPDATE threads SET status = ?, error = ?, updated_at_ms = ? WHERE id = ?`,
		status, nullString(errorMessage), timeToMs(state.UpdatedAt), id)
	return err
}

func (s *Store) AddUsage(id string, usage storage.Usage) error {
	if usage.InputTokens == 0 && usage.CachedInputTokens == 0 && usage.OutputTokens == 0 && usage.ReasoningOutputTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	s.mu.Lock()
	total := usage.TotalTokens
	if total == 0 {
		total = usage.InputTokens + usage.OutputTokens
	}
	_, err := s.db.Exec(`UPDATE threads SET
input_tokens = input_tokens + ?,
cached_input_tokens = cached_input_tokens + ?,
output_tokens = output_tokens + ?,
reasoning_output_tokens = reasoning_output_tokens + ?,
total_tokens = total_tokens + ?,
updated_at_ms = ?
WHERE id = ?`,
		usage.InputTokens, usage.CachedInputTokens, usage.OutputTokens, usage.ReasoningOutputTokens, total, timeToMs(time.Now().UTC()), id)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if session, err := s.LoadSession(id); err == nil {
		s.mirrorSession(session)
	}
	return nil
}

func (s *Store) mirrorSession(session storage.Session) {
	if s.mirror != nil {
		_ = s.mirror.SaveSession(session)
	}
}

func (s *Store) mirrorMessages(id string, messages []provider.Message) {
	if s.mirror != nil {
		_ = s.mirror.SaveMessages(id, messages)
	}
}

func (s *Store) configure() error {
	for _, stmt := range []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS threads (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT,
  parent_id TEXT,
  status TEXT NOT NULL DEFAULT 'idle',
  error TEXT,
  runtime TEXT NOT NULL DEFAULT 'native',
  acp_agent TEXT,
  acp_session_id TEXT,
  cwd TEXT,
  model_provider TEXT,
  model TEXT,
  reasoning_effort TEXT,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  archived INTEGER NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS messages (
  thread_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  reasoning TEXT,
  blocks TEXT NOT NULL DEFAULT '[]',
  created_at_ms INTEGER NOT NULL,
  PRIMARY KEY (thread_id, seq),
  FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
)`,
		`CREATE TABLE IF NOT EXISTS session_events (
  thread_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  type TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  acp TEXT,
  permission TEXT,
  created_at_ms INTEGER NOT NULL,
  PRIMARY KEY (thread_id, seq),
  FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
)`,
		`CREATE INDEX IF NOT EXISTS idx_threads_parent_updated ON threads(parent_id, updated_at_ms DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_threads_updated ON threads(updated_at_ms DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_session_events_thread_seq ON session_events(thread_id, seq)`,
		`PRAGMA user_version = 1`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	// Tolerant migration for databases created before the column existed
	// (CREATE TABLE IF NOT EXISTS skips them).
	if _, err := s.db.Exec(`ALTER TABLE threads ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE threads ADD COLUMN error TEXT`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE threads ADD COLUMN cwd TEXT`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	return nil
}

func (s *Store) importLegacyJSON() error {
	legacy := s.mirror
	if legacy == nil {
		var err error
		legacy, err = jsonstore.New(s.root)
		if err != nil {
			return err
		}
	}
	sessions, err := legacy.ListSessions(storage.SessionFilter{IncludeChildren: true})
	if err != nil {
		return err
	}
	for _, session := range sessions {
		var existing string
		err := s.db.QueryRow(`SELECT id FROM threads WHERE id = ?`, session.ID).Scan(&existing)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		if session.Status == "" {
			session.Status = storage.StatusIdle
		}
		if session.Runtime == "" {
			session.Runtime = storage.RuntimeNative
		}
		slug, err := s.uniqueSlugLocked(session.Slug, session.ID)
		if err != nil {
			return err
		}
		session.Slug = slug
		messages, err := legacy.LoadMessages(session.ID)
		if err != nil {
			return err
		}
		records, err := recordsFromProviderMessages(messages, session.CreatedAt)
		if err != nil {
			return fmt.Errorf("import legacy session %s: %w", session.ID, err)
		}
		tx, err := s.db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		if err := insertSession(tx, session); err != nil {
			_ = tx.Rollback()
			return err
		}
		for _, record := range records {
			record.ThreadID = session.ID
			if err := insertMessage(tx, record); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) resetStaleRunningThreads() error {
	_, err := s.db.Exec(`UPDATE threads SET status = ?, error = ?, updated_at_ms = ? WHERE status = ?`,
		storage.StatusError, "Server restarted while this thread was still running.", timeToMs(time.Now().UTC()), storage.StatusRunning)
	return err
}

func (s *Store) backfillMissingThreadErrors() error {
	rows, err := s.db.Query(`SELECT id FROM threads WHERE status = ? AND (error IS NULL OR error = '')`, storage.StatusError)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		records, err := s.loadMessageRecordsLocked(id)
		if err != nil {
			return err
		}
		if message := sessionErrorFromRecords(records); message != "" {
			if _, err := s.db.Exec(`UPDATE threads SET error = ? WHERE id = ?`, message, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func sessionErrorFromRecords(records []storage.Message) string {
	for i := len(records) - 1; i >= 0; i-- {
		blocks := records[i].Blocks
		for j := len(blocks) - 1; j >= 0; j-- {
			block := blocks[j]
			if block.Type != "tool" || strings.TrimSpace(block.Result) == "" {
				continue
			}
			var parsed struct {
				Error  string `json:"error"`
				Status string `json:"status"`
			}
			if err := stdjson.Unmarshal([]byte(block.Result), &parsed); err != nil {
				continue
			}
			if parsed.Error == "" || (parsed.Status != "" && parsed.Status != storage.StatusError) {
				continue
			}
			if block.Name != "" {
				return fmt.Sprintf("%s failed: %s", block.Name, parsed.Error)
			}
			return parsed.Error
		}
	}
	return ""
}

func (s *Store) loadSessionLocked(ref string) (storage.Session, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.Session{}, fmt.Errorf("session id or slug is required")
	}
	row := s.db.QueryRow(`SELECT id, slug, title, parent_id, status, error, runtime, acp_agent, acp_session_id, cwd,
model_provider, model, reasoning_effort, input_tokens, cached_input_tokens, output_tokens,
reasoning_output_tokens, total_tokens, archived, created_at_ms, updated_at_ms
FROM threads WHERE id = ? OR slug = ? LIMIT 1`, ref, ref)
	return scanSession(row)
}

func (s *Store) saveSessionLocked(session storage.Session, touchUpdated bool) error {
	if session.ID == "" {
		return fmt.Errorf("session id is empty")
	}
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeNative
	}
	if session.Status == "" {
		session.Status = storage.StatusIdle
	}
	if session.Status != storage.StatusError {
		session.Error = ""
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	if touchUpdated || session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	}
	slug, err := s.uniqueSlugLocked(session.Slug, session.ID)
	if err != nil {
		return err
	}
	session.Slug = slug
	return insertSession(s.db, session)
}

func (s *Store) loadMessageRecordsLocked(id string) ([]storage.Message, error) {
	rows, err := s.db.Query(`SELECT thread_id, seq, role, content, reasoning, blocks, created_at_ms
FROM messages WHERE thread_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []storage.Message
	for rows.Next() {
		var record storage.Message
		var reasoning sql.NullString
		var rawBlocks string
		var createdMs int64
		if err := rows.Scan(&record.ThreadID, &record.Seq, &record.Role, &record.Content, &reasoning, &rawBlocks, &createdMs); err != nil {
			return nil, err
		}
		record.Reasoning = reasoning.String
		blocks, err := unmarshalBlocks(rawBlocks)
		if err != nil {
			return nil, err
		}
		record.Blocks = blocks
		record.CreatedAt = msToTime(createdMs)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) replaceMessagesLocked(id string, records []storage.Message) error {
	existing, err := s.loadMessageRecordsLocked(id)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM messages WHERE thread_id = ?`, id); err != nil {
		return err
	}
	now := time.Now().UTC()
	for i, record := range records {
		record.ThreadID = id
		record.Seq = int64(i + 1)
		// Already-stored rows keep their timestamps; only new rows are stamped.
		if i < len(existing) && existing[i].Role == record.Role {
			record.CreatedAt = existing[i].CreatedAt
		} else if record.CreatedAt.IsZero() {
			record.CreatedAt = now.Add(time.Duration(i+1) * time.Millisecond)
		}
		if err := insertMessage(tx, record); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE threads SET updated_at_ms = ? WHERE id = ?`, timeToMs(now), id); err != nil {
		return err
	}
	return tx.Commit()
}

type execer interface {
	Exec(string, ...any) (sql.Result, error)
}

func insertSession(db execer, session storage.Session) error {
	acpAgent, acpSessionID, cwd := runtimeRefColumns(session)
	_, err := db.Exec(`INSERT INTO threads (
id, slug, title, parent_id, status, runtime, acp_agent, acp_session_id, cwd,
error, model_provider, model, reasoning_effort, input_tokens, cached_input_tokens, output_tokens,
reasoning_output_tokens, total_tokens, archived, created_at_ms, updated_at_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
slug = excluded.slug,
title = excluded.title,
parent_id = excluded.parent_id,
status = excluded.status,
error = excluded.error,
runtime = excluded.runtime,
acp_agent = excluded.acp_agent,
acp_session_id = excluded.acp_session_id,
cwd = excluded.cwd,
model_provider = excluded.model_provider,
model = excluded.model,
reasoning_effort = excluded.reasoning_effort,
input_tokens = excluded.input_tokens,
cached_input_tokens = excluded.cached_input_tokens,
output_tokens = excluded.output_tokens,
reasoning_output_tokens = excluded.reasoning_output_tokens,
total_tokens = excluded.total_tokens,
archived = excluded.archived,
created_at_ms = excluded.created_at_ms,
updated_at_ms = excluded.updated_at_ms`,
		session.ID, session.Slug, nullString(session.Title), nullString(session.ParentID),
		session.Status, session.Runtime, nullString(acpAgent), nullString(acpSessionID), nullString(cwd), nullString(session.Error),
		nullString(session.ModelProvider), nullString(session.Model), nullString(session.ReasoningEffort),
		session.Usage.InputTokens, session.Usage.CachedInputTokens, session.Usage.OutputTokens,
		session.Usage.ReasoningOutputTokens, session.Usage.TotalTokens, session.Archived,
		timeToMs(session.CreatedAt), timeToMs(session.UpdatedAt))
	return err
}

func insertMessage(db execer, record storage.Message) error {
	blocks, err := marshalBlocks(record.Blocks)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO messages (thread_id, seq, role, content, reasoning, blocks, created_at_ms)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		record.ThreadID, record.Seq, record.Role, record.Content, nullString(record.Reasoning), blocks, timeToMs(record.CreatedAt))
	return err
}


type sessionScanner interface {
	Scan(...any) error
}

func scanSession(scanner sessionScanner) (storage.Session, error) {
	var session storage.Session
	var title, parentID, errorMessage, acpAgent, acpSessionID, cwd, modelProvider, model, reasoningEffort sql.NullString
	var createdMs, updatedMs int64
	err := scanner.Scan(&session.ID, &session.Slug, &title, &parentID, &session.Status, &errorMessage, &session.Runtime,
		&acpAgent, &acpSessionID, &cwd, &modelProvider, &model, &reasoningEffort,
		&session.Usage.InputTokens, &session.Usage.CachedInputTokens, &session.Usage.OutputTokens,
		&session.Usage.ReasoningOutputTokens, &session.Usage.TotalTokens, &session.Archived, &createdMs, &updatedMs)
	if err != nil {
		return storage.Session{}, err
	}
	session.Title = title.String
	session.ParentID = parentID.String
	session.Error = errorMessage.String
	session.ModelProvider = modelProvider.String
	session.Model = model.String
	session.ReasoningEffort = reasoningEffort.String
	session.CreatedAt = msToTime(createdMs)
	session.UpdatedAt = msToTime(updatedMs)
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeNative
	}
	if session.Status == "" {
		session.Status = storage.StatusIdle
	}
	if acpAgent.Valid || acpSessionID.Valid || cwd.Valid {
		session.RuntimeRef = &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     acpAgent.String,
			SessionID: acpSessionID.String,
			Cwd:       cwd.String,
		}
	}
	return session, nil
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

func defaultSlug(session storage.Session) string {
	if session.Title != "" {
		return session.Title
	}
	if session.Runtime == storage.RuntimeACP && session.RuntimeRef != nil && session.RuntimeRef.Agent != "" {
		return session.RuntimeRef.Agent
	}
	return "chat-" + time.Now().UTC().Format("2006-01-02-150405")
}

func normalizeSlug(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = slugUnsafe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "session"
	}
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	return slug
}

func (s *Store) uniqueSlugLocked(value, currentID string) (string, error) {
	base := normalizeSlug(value)
	slug := base
	for i := 2; ; i++ {
		var found string
		err := s.db.QueryRow(`SELECT id FROM threads WHERE slug = ? LIMIT 1`, slug).Scan(&found)
		if err == sql.ErrNoRows || found == currentID {
			return slug, nil
		}
		if err != nil {
			return "", err
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

func runtimeRefColumns(session storage.Session) (string, string, string) {
	if session.RuntimeRef == nil {
		return "", "", ""
	}
	return session.RuntimeRef.Agent, session.RuntimeRef.SessionID, session.RuntimeRef.Cwd
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func timeToMs(t time.Time) int64 {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UnixMilli()
}

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
