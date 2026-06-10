package sqlite

import (
	"context"
	"database/sql"
	stdjson "encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) CreateSession(input storage.CreateSession) (storage.Session, error) {
	s.mu.Lock()

	now := time.Now().UTC()
	session := storage.Session{
		ID:              s.NewSessionID(),
		Slug:            input.Slug,
		Title:           input.Title,
		ParentID:        input.ParentID,
		Status:          storage.StatusIdle,
		Runtime:         firstNonEmpty(input.Runtime, storage.RuntimeNative),
		RuntimeRef:      input.RuntimeRef,
		ModelProvider:   input.ModelProvider,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		SourceType:      input.SourceType,
		SourceID:        input.SourceID,
		CreatedAt:       now,
		UpdatedAt:       now,
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
	return threaddb.New(s.db).SetArchived(context.Background(), threaddb.SetArchivedParams{
		Archived: boolInt(archived),
		ID:       id,
	})
}

func (s *Store) ListSessions(filter storage.SessionFilter) ([]storage.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := threaddb.New(s.db).ListSessions(context.Background())
	if err != nil {
		return nil, err
	}

	var sessions []storage.Session
	for _, row := range rows {
		session, err := sessionFromDB(row)
		if err != nil {
			return nil, err
		}
		if !storage.SessionMatchesFilter(session, filter) {
			continue
		}
		sessions = append(sessions, session)
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

func (s *Store) loadSessionLocked(ref string) (storage.Session, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.Session{}, fmt.Errorf("session id or slug is required")
	}
	row, err := threaddb.New(s.db).GetSession(context.Background(), ref)
	if err != nil {
		return storage.Session{}, err
	}
	return sessionFromDB(row)
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

func insertSession(db threaddb.DBTX, session storage.Session) error {
	acpAgent, acpSessionID, cwd := runtimeRefColumns(session)
	queuedMessages, err := marshalStringList(session.QueuedMessages)
	if err != nil {
		return err
	}
	return threaddb.New(db).UpsertSession(context.Background(), threaddb.UpsertSessionParams{
		ID:                    session.ID,
		Slug:                  session.Slug,
		Title:                 nullDBString(session.Title),
		ParentID:              nullDBString(session.ParentID),
		Status:                session.Status,
		Runtime:               session.Runtime,
		AcpAgent:              nullDBString(acpAgent),
		AcpSessionID:          nullDBString(acpSessionID),
		Cwd:                   nullDBString(cwd),
		Error:                 nullDBString(session.Error),
		ModelProvider:         nullDBString(session.ModelProvider),
		Model:                 nullDBString(session.Model),
		ReasoningEffort:       nullDBString(session.ReasoningEffort),
		InputTokens:           session.Usage.InputTokens,
		CachedInputTokens:     session.Usage.CachedInputTokens,
		CachedWriteTokens:     session.Usage.CachedWriteTokens,
		OutputTokens:          session.Usage.OutputTokens,
		ReasoningOutputTokens: session.Usage.ReasoningOutputTokens,
		TotalTokens:           session.Usage.TotalTokens,
		ContextTokens:         session.Usage.ContextTokens,
		ContextWindowTokens:   session.Usage.ContextWindowTokens,
		QueuedMessages:        queuedMessages,
		SourceType:            nullDBString(session.SourceType),
		SourceID:              nullDBString(session.SourceID),
		Archived:              boolInt(session.Archived),
		CreatedAtMs:           timeToMs(session.CreatedAt),
		UpdatedAtMs:           timeToMs(session.UpdatedAt),
	})
}

func sessionFromDB(row threaddb.Thread) (storage.Session, error) {
	queuedMessages, err := unmarshalStringList(row.QueuedMessages)
	if err != nil {
		return storage.Session{}, err
	}
	session := storage.Session{
		ID:              row.ID,
		Slug:            row.Slug,
		Title:           row.Title.String,
		ParentID:        row.ParentID.String,
		Status:          row.Status,
		Error:           row.Error.String,
		Runtime:         row.Runtime,
		ModelProvider:   row.ModelProvider.String,
		Model:           row.Model.String,
		ReasoningEffort: row.ReasoningEffort.String,
		Usage: storage.Usage{
			InputTokens:           row.InputTokens,
			CachedInputTokens:     row.CachedInputTokens,
			CachedWriteTokens:     row.CachedWriteTokens,
			OutputTokens:          row.OutputTokens,
			ReasoningOutputTokens: row.ReasoningOutputTokens,
			TotalTokens:           row.TotalTokens,
			ContextTokens:         row.ContextTokens,
			ContextWindowTokens:   row.ContextWindowTokens,
		},
		QueuedMessages: queuedMessages,
		SourceType:     row.SourceType.String,
		SourceID:       row.SourceID.String,
		Archived:       row.Archived != 0,
		CreatedAt:      msToTime(row.CreatedAtMs),
		UpdatedAt:      msToTime(row.UpdatedAtMs),
	}
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeNative
	}
	if session.Status == "" {
		session.Status = storage.StatusIdle
	}
	if row.AcpAgent.Valid || row.AcpSessionID.Valid || row.Cwd.Valid {
		session.RuntimeRef = &storage.RuntimeRef{
			Type:      session.Runtime,
			Agent:     row.AcpAgent.String,
			SessionID: row.AcpSessionID.String,
			Cwd:       row.Cwd.String,
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
		found, err := threaddb.New(s.db).GetThreadIDBySlug(context.Background(), slug)
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

func marshalStringList(values []string) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	data, err := stdjson.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalStringList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var values []string
	if err := stdjson.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
