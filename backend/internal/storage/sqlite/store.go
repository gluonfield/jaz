package sqlite

import (
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/search"
	_ "modernc.org/sqlite"
)

type Store struct {
	root           string
	secrets        cipher.AEAD
	db             *sql.DB
	searchQueries  search.Querier
	exportMirror   sessionExportMirror
	writeMu        writeMutex
	eventPlanner   eventCompactionPlanner
	compactionWake chan struct{}
}

type writeMutex struct {
	mu      sync.Mutex
	waiting atomic.Int64
}

func (m *writeMutex) Lock() {
	m.waiting.Add(1)
	m.mu.Lock()
	m.waiting.Add(-1)
}

func (m *writeMutex) Unlock() {
	m.mu.Unlock()
}

func (m *writeMutex) tryMaintenanceLock() bool {
	if m.waiting.Load() > 0 || !m.mu.TryLock() {
		return false
	}
	if m.waiting.Load() == 0 {
		return true
	}
	m.mu.Unlock()
	return false
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
	store := &Store{root: root, compactionWake: make(chan struct{}, 1)}
	for _, dir := range []string{store.RootDir(), store.SessionsDir(), store.WorkspacesDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, err
	}
	for _, name := range []string{"jaz.sqlite", "jaz.sqlite-wal", "jaz.sqlite-shm"} {
		file, err := os.OpenFile(filepath.Join(root, name), os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, err
		}
		err = file.Chmod(0o600)
		_ = file.Close()
		if err != nil {
			return nil, err
		}
	}
	exportMirror, err := jsonstore.New(root)
	if err != nil {
		return nil, err
	}
	store.exportMirror = exportMirror
	db, err := sql.Open("sqlite", filepath.Join(root, "jaz.sqlite")+"?_pragma=secure_delete(ON)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	store.db = db
	store.searchQueries = search.New(db)
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.protectSecrets(); err != nil {
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
