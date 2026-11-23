package instructions

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

// DBStore stores user AGENTS instructions in SQLite (with a one-time read
// import from legacy <root>/AGENTS.md on first access) and delegates bot-scoped
// files to fallback.
type DBStore struct {
	db            storage.LocalInstructionStore
	legacyRoot    string
	fallback      Store
	mu            sync.Mutex
	legacyChecked bool
}

var _ Store = (*DBStore)(nil)

func NewDBStore(db storage.LocalInstructionStore, legacyRoot string, fallback Store) *DBStore {
	return &DBStore{
		db:         db,
		legacyRoot: strings.TrimSpace(legacyRoot),
		fallback:   fallback,
	}
}

func (s *DBStore) handlesInDB(key Key) bool {
	return key.Scope == ScopeUser && key.Kind == KindAgents
}

func (s *DBStore) Get(ctx context.Context, key Key) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if err := key.Validate(); err != nil {
		return Document{}, err
	}
	if !s.handlesInDB(key) {
		if s.fallback == nil {
			return Document{}, ErrUnsupportedScope
		}
		return s.fallback.Get(ctx, key)
	}
	rec, found, err := s.db.GetLocalInstruction(ctx, string(key.Scope), key.ScopeID, string(key.Kind))
	if err != nil {
		return Document{}, err
	}
	if found {
		return docFromLocalRecord(key, rec), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.legacyChecked {
		return Document{Key: key}, nil
	}
	return s.ensureLegacyImportedLocked(ctx, key)
}

func (s *DBStore) Save(ctx context.Context, key Key, content, expectedVersion string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if err := key.Validate(); err != nil {
		return Document{}, err
	}
	if !s.handlesInDB(key) {
		if s.fallback == nil {
			return Document{}, ErrUnsupportedScope
		}
		return s.fallback.Save(ctx, key, content, expectedVersion)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.legacyChecked {
		if _, err := s.ensureLegacyImportedLocked(ctx, key); err != nil {
			return Document{}, err
		}
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	saved, conflict, err := s.db.SaveLocalInstruction(ctx, storage.LocalInstruction{
		Scope:     string(key.Scope),
		ScopeID:   key.ScopeID,
		Kind:      string(key.Kind),
		BodyMD:    content,
		Version:   Digest([]byte(content)),
		UpdatedAt: now,
	}, expectedVersion)
	if err != nil {
		return Document{}, err
	}
	doc := docFromLocalRecord(key, saved)
	if conflict {
		return doc, ErrConflict
	}
	return doc, nil
}

func (s *DBStore) ensureLegacyImportedLocked(ctx context.Context, key Key) (Document, error) {
	rec, found, err := s.db.GetLocalInstruction(ctx, string(key.Scope), key.ScopeID, string(key.Kind))
	if err != nil {
		return Document{}, err
	}
	if found {
		s.legacyChecked = true
		return docFromLocalRecord(key, rec), nil
	}
	if s.legacyRoot == "" {
		s.legacyChecked = true
		return Document{Key: key}, nil
	}
	path := filepath.Join(s.legacyRoot, "AGENTS.md")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		s.legacyChecked = true
		return Document{Key: key}, nil
	}
	if err != nil {
		return Document{}, err
	}
	updatedAt := time.Now().UTC().Truncate(time.Millisecond)
	if info, statErr := os.Stat(path); statErr == nil {
		updatedAt = info.ModTime().UTC().Truncate(time.Millisecond)
	}
	imported, err := s.db.ImportLocalInstruction(ctx, storage.LocalInstruction{
		Scope:     string(key.Scope),
		ScopeID:   key.ScopeID,
		Kind:      string(key.Kind),
		BodyMD:    string(data),
		Version:   Digest(data),
		UpdatedAt: updatedAt,
	})
	if err != nil {
		return Document{}, err
	}
	s.legacyChecked = true
	return docFromLocalRecord(key, imported), nil
}

func docFromLocalRecord(key Key, rec storage.LocalInstruction) Document {
	return Document{
		Key:       key,
		Content:   rec.BodyMD,
		Version:   rec.Version,
		UpdatedAt: rec.UpdatedAt,
	}
}
