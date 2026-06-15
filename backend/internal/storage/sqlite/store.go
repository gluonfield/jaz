package sqlite

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/searchdb"
	_ "modernc.org/sqlite"
)

type Store struct {
	root          string
	db            *sql.DB
	searchQueries searchdb.Querier
	mirror        *jsonstore.Store
	mu            sync.Mutex
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
	store.searchQueries = searchdb.New(db)
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
