package jsonstore

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	stdjson "encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

type Store struct {
	root string
	mu   sync.Mutex
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
	for _, dir := range []string{store.SessionsDir(), store.WorkspacesDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return store, nil
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
	now := time.Now().UTC()
	runtime := input.Runtime
	if runtime == "" {
		runtime = storage.RuntimeNative
	}
	session := storage.Session{
		ID:              s.NewSessionID(),
		Slug:            input.Slug,
		Title:           input.Title,
		ParentID:        input.ParentID,
		Status:          storage.StatusIdle,
		Runtime:         runtime,
		RuntimeRef:      input.RuntimeRef,
		ModelProvider:   input.ModelProvider,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		SourceType:      input.SourceType,
		SourceID:        input.SourceID,
		CreatedAt:       now,
		UpdatedAt:       now,
		LastAttentionAt: now,
	}
	if session.Slug == "" {
		session.Slug = defaultSlug(session)
	}
	slug, err := s.uniqueSlug(session.Slug, "")
	if err != nil {
		return storage.Session{}, err
	}
	session.Slug = slug
	if err := s.SaveSession(session); err != nil {
		return storage.Session{}, err
	}
	return session, nil
}

func (s *Store) EnsureSession(id string) error {
	if id == "" {
		return fmt.Errorf("session id is empty")
	}
	return os.MkdirAll(s.sessionDir(id), 0o755)
}

func (s *Store) LoadSession(ref string) (storage.Session, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.Session{}, fmt.Errorf("session id or slug is required")
	}
	if session, err := s.loadSessionByID(ref); err == nil {
		return session, nil
	}
	sessions, err := s.ListSessions(storage.SessionFilter{IncludeChildren: true})
	if err != nil {
		return storage.Session{}, err
	}
	var matches []storage.Session
	for _, session := range sessions {
		if session.Slug == ref {
			matches = append(matches, session)
		}
	}
	if len(matches) == 0 {
		return storage.Session{}, fmt.Errorf("session not found: %s", ref)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, session := range matches {
			ids = append(ids, session.ID)
		}
		return storage.Session{}, fmt.Errorf("ambiguous session slug %q: %s", ref, strings.Join(ids, ", "))
	}
	return matches[0], nil
}

func (s *Store) SaveSession(session storage.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSession(session)
}

func (s *Store) saveSession(session storage.Session) error {
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
	if session.LastAttentionAt.IsZero() {
		storage.MarkSessionAttention(&session, storage.SessionAttentionAt(session))
	}
	session.UpdatedAt = time.Now().UTC()
	slug, err := s.uniqueSlug(session.Slug, session.ID)
	if err != nil {
		return err
	}
	session.Slug = slug
	if err := s.EnsureSession(session.ID); err != nil {
		return err
	}
	data, err := stdjson.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaPath(session.ID), data, 0o644)
}

// SetArchived archives or restores a session together with its children.
func (s *Store) SetArchived(id string, archived bool) error {
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	session.Archived = archived
	if err := s.saveSession(session); err != nil {
		return err
	}
	children, err := s.ListSessions(storage.SessionFilter{ParentID: id, ParentOnly: true, Archived: !archived})
	if err != nil {
		return err
	}
	for _, child := range children {
		child.Archived = archived
		if err := s.saveSession(child); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SetPinned(id string, pinned bool) error {
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	session.Pinned = pinned
	if err := s.saveSession(session); err != nil {
		return err
	}
	children, err := s.ListSessions(storage.SessionFilter{ParentID: id, ParentOnly: true})
	if err != nil {
		return err
	}
	for _, child := range children {
		child.Pinned = pinned
		if err := s.saveSession(child); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListSessions(filter storage.SessionFilter) ([]storage.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listSessionsLocked(filter)
}

func (s *Store) listSessionsLocked(filter storage.SessionFilter) ([]storage.Session, error) {
	entries, err := os.ReadDir(s.SessionsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sessions := make([]storage.Session, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		session, err := s.loadSessionByID(entry.Name())
		if err != nil {
			continue
		}
		if !storage.SessionMatchesFilter(session, filter) {
			continue
		}
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		left := storage.SessionAttentionAt(sessions[i])
		right := storage.SessionAttentionAt(sessions[j])
		if left.Equal(right) {
			return sessions[i].ID < sessions[j].ID
		}
		return left.After(right)
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

func (s *Store) TouchSessionAttention(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	session.UpdatedAt = now
	storage.MarkSessionAttention(&session, now)
	return s.saveSession(session)
}

func (s *Store) AddUsage(id string, usage storage.Usage) error {
	if usage.IsZero() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.loadSessionByID(id)
	if err != nil {
		return err
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.ComponentTotal()
	}
	session.Usage.InputTokens += usage.InputTokens
	session.Usage.CachedInputTokens += usage.CachedInputTokens
	session.Usage.CachedWriteTokens += usage.CachedWriteTokens
	session.Usage.OutputTokens += usage.OutputTokens
	session.Usage.ReasoningOutputTokens += usage.ReasoningOutputTokens
	session.Usage.TotalTokens += total
	if context := usage.LiveContextTokens(); context > 0 {
		session.Usage.ContextTokens = context
	}
	if usage.ContextWindowTokens > 0 {
		session.Usage.ContextWindowTokens = usage.ContextWindowTokens
	}
	if err := s.saveSession(session); err != nil {
		return err
	}
	return s.appendUsageEvent(session, usage, total, usage.LiveContextTokens(), time.Now().UTC())
}

func (s *Store) LoadMessages(id string) ([]provider.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadMessages(id)
}

func (s *Store) loadMessages(id string) ([]provider.Message, error) {
	path := filepath.Join(s.sessionDir(id), "messages.jsonl")
	data, err := os.ReadFile(path)
	if err == nil {
		return unmarshalMessagesJSONL(data)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	path = filepath.Join(s.sessionDir(id), "messages.json")
	data, err = os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var messages []provider.Message
	if err := stdjson.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *Store) SaveMessages(id string, messages []provider.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveMessages(id, messages)
}

func (s *Store) AppendMessages(id string, messages ...provider.Message) error {
	if len(messages) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.EnsureSession(id); err != nil {
		return err
	}
	path := filepath.Join(s.sessionDir(id), "messages.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		existing, err := s.loadMessages(id)
		if err != nil {
			return err
		}
		return s.saveMessages(id, append(existing, messages...))
	} else if err != nil {
		return err
	}
	if err := appendMessagesJSONL(path, messages); err != nil {
		return err
	}
	if err := removeLegacyMessagesJSON(s.sessionDir(id)); err != nil {
		return err
	}
	s.touchSession(id)
	return nil
}

func (s *Store) saveMessages(id string, messages []provider.Message) error {
	if err := s.EnsureSession(id); err != nil {
		return err
	}
	lines, err := marshalMessagesJSONL(messages)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.sessionDir(id), "messages.jsonl"), lines, 0o644); err != nil {
		return err
	}
	if err := removeLegacyMessagesJSON(s.sessionDir(id)); err != nil {
		return err
	}
	s.touchSession(id)
	return nil
}

func unmarshalMessagesJSONL(data []byte) ([]provider.Message, error) {
	dec := stdjson.NewDecoder(bytes.NewReader(data))
	var messages []provider.Message
	for {
		var message provider.Message
		if err := dec.Decode(&message); err != nil {
			if err == io.EOF {
				return messages, nil
			}
			return nil, err
		}
		messages = append(messages, message)
	}
}

func marshalMessagesJSONL(messages []provider.Message) ([]byte, error) {
	var out bytes.Buffer
	enc := stdjson.NewEncoder(&out)
	for _, message := range messages {
		if err := enc.Encode(message); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func appendMessagesJSONL(path string, messages []provider.Message) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := stdjson.NewEncoder(file)
	for _, message := range messages {
		if err := enc.Encode(message); err != nil {
			return err
		}
	}
	return nil
}

func removeLegacyMessagesJSON(dir string) error {
	err := os.Remove(filepath.Join(dir, "messages.json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Store) touchSession(id string) {
	if session, err := s.loadSessionByID(id); err == nil {
		session.UpdatedAt = time.Now().UTC()
		_ = s.saveSession(session)
	}
}

func (s *Store) LoadActivity(id string) ([]storage.ActivityEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadActivity(id)
}

func (s *Store) loadActivity(id string) ([]storage.ActivityEntry, error) {
	path := filepath.Join(s.sessionDir(id), "activity.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var activity []storage.ActivityEntry
	if err := stdjson.Unmarshal(data, &activity); err != nil {
		return nil, err
	}
	return activity, nil
}

func (s *Store) SaveActivity(id string, activity []storage.ActivityEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveActivity(id, activity)
}

func (s *Store) UpsertActivity(id string, entry storage.ActivityEntry) error {
	if entry.At.IsZero() {
		entry.At = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	activity, err := s.loadActivity(id)
	if err != nil {
		return err
	}
	if entry.ID != "" {
		for i := range activity {
			if activity[i].ID == entry.ID {
				activity[i] = entry
				return s.saveActivity(id, activity)
			}
		}
	}
	return s.saveActivity(id, append(activity, entry))
}

func (s *Store) saveActivity(id string, activity []storage.ActivityEntry) error {
	if err := s.EnsureSession(id); err != nil {
		return err
	}
	data, err := stdjson.MarshalIndent(activity, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.sessionDir(id), "activity.json"), data, 0o644); err != nil {
		return err
	}
	if session, err := s.loadSessionByID(id); err == nil {
		session.UpdatedAt = time.Now().UTC()
		_ = s.saveSession(session)
	}
	return nil
}

func (s *Store) LoadACPState(id string) (storage.ACPState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.sessionDir(id), "acp_state.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return storage.ACPState{}, fmt.Errorf("acp state not found: %s", id)
	}
	if err != nil {
		return storage.ACPState{}, err
	}
	var state storage.ACPState
	if err := stdjson.Unmarshal(data, &state); err != nil {
		return storage.ACPState{}, err
	}
	return state, nil
}

func (s *Store) SaveACPState(id string, state storage.ACPState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.EnsureSession(id); err != nil {
		return err
	}
	if state.ID == "" {
		state.ID = id
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	data, err := stdjson.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.sessionDir(id), "acp_state.json"), data, 0o644); err != nil {
		return err
	}
	if session, err := s.loadSessionByID(id); err == nil {
		session.UpdatedAt = state.UpdatedAt
		if status := storage.SessionStatusForACPState(state.State); status != "" {
			session.Status = status
			if status == storage.StatusError {
				session.Error = state.Error
			} else {
				session.Error = ""
			}
		}
		_ = s.saveSession(session)
	}
	return nil
}

func (s *Store) sessionDir(id string) string {
	return filepath.Join(s.SessionsDir(), id)
}

func (s *Store) metaPath(id string) string {
	return filepath.Join(s.sessionDir(id), "meta.json")
}

func (s *Store) loadSessionByID(id string) (storage.Session, error) {
	data, err := os.ReadFile(s.metaPath(id))
	if os.IsNotExist(err) {
		return storage.Session{}, fmt.Errorf("session metadata not found: %s", id)
	}
	if err != nil {
		return storage.Session{}, err
	}
	var session storage.Session
	if err := stdjson.Unmarshal(data, &session); err != nil {
		return storage.Session{}, err
	}
	if session.Status == "" {
		session.Status = storage.StatusIdle
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

func (s *Store) uniqueSlug(value, currentID string) (string, error) {
	base := normalizeSlug(value)
	slug := base
	for i := 2; ; i++ {
		exists, err := s.slugExists(slug, currentID)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

func (s *Store) slugExists(slug, currentID string) (bool, error) {
	entries, err := os.ReadDir(s.SessionsDir())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == currentID {
			continue
		}
		session, err := s.loadSessionByID(entry.Name())
		if err != nil {
			continue
		}
		if session.Slug == slug {
			return true, nil
		}
	}
	return false, nil
}
