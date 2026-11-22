package instructions

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wins/jaz/backend/internal/pathsafe"
)

type LocalFSOption func(*LocalFSStore)

type LocalFSStore struct {
	root           string
	botsRoot       string
	resolveBotHome func(botID string) (string, error)
	mu             sync.Mutex
}

var _ Store = (*LocalFSStore)(nil)

func WithBotsRoot(dir string) LocalFSOption {
	return func(s *LocalFSStore) {
		s.botsRoot = strings.TrimSpace(dir)
	}
}

func WithBotHomeResolver(fn func(botID string) (string, error)) LocalFSOption {
	return func(s *LocalFSStore) {
		s.resolveBotHome = fn
	}
}

func NewLocalFSStore(root string, opts ...LocalFSOption) *LocalFSStore {
	root = strings.TrimSpace(root)
	store := &LocalFSStore{
		root: root,
	}
	if root != "" {
		store.botsRoot = filepath.Join(root, "bots")
	}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	return store
}

func (s *LocalFSStore) Get(ctx context.Context, key Key) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	path, ok, err := s.resolvePath(key)
	if err != nil {
		return Document{}, err
	}
	if !ok {
		return Document{Key: key}, nil
	}
	return readDocument(key, path)
}

func (s *LocalFSStore) Save(ctx context.Context, key Key, content, expectedVersion string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	path, ok, err := s.resolvePath(key)
	if err != nil {
		return Document{}, err
	}
	if !ok {
		return Document{}, fmt.Errorf("instruction store root is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := readDocument(key, path)
	if err != nil {
		return Document{}, err
	}
	if current.Version != expectedVersion {
		return current, ErrConflict
	}
	if err := replaceFile(path, content); err != nil {
		return Document{}, err
	}
	return readDocument(key, path)
}

func (s *LocalFSStore) resolvePath(key Key) (string, bool, error) {
	if err := key.Validate(); err != nil {
		return "", false, err
	}
	switch key.Scope {
	case ScopeUser:
		if s.root == "" {
			return "", false, nil
		}
		name, err := userFileName(key.Kind)
		if err != nil {
			return "", false, err
		}
		return filepath.Join(s.root, name), true, nil
	case ScopeBot:
		if key.ScopeID == "" {
			return "", false, ErrInvalidKey
		}
		name, err := botFileName(key.Kind)
		if err != nil {
			return "", false, err
		}
		home, err := s.botHome(key.ScopeID)
		if err != nil {
			return "", false, err
		}
		return filepath.Join(home, name), true, nil
	default:
		return "", false, ErrInvalidKey
	}
}

func (s *LocalFSStore) botHome(botID string) (string, error) {
	if s.resolveBotHome != nil {
		home, err := s.resolveBotHome(botID)
		if err != nil {
			return "", err
		}
		if home == "" {
			return "", fmt.Errorf("%s has no home", botID)
		}
		return home, nil
	}
	if s.botsRoot == "" {
		return "", fmt.Errorf("bot home root is not configured")
	}
	if strings.ContainsAny(botID, "/\\\x00") || botID == "." || botID == ".." {
		return "", ErrInvalidKey
	}
	return pathsafe.Resolve(s.botsRoot, botID)
}

func userFileName(kind Kind) (string, error) {
	switch kind {
	case KindAgents:
		return "AGENTS.md", nil
	case KindSoul:
		return "SOUL.md", nil
	case KindBots:
		return "BOTS.md", nil
	default:
		return "", ErrInvalidKey
	}
}

func botFileName(kind Kind) (string, error) {
	switch kind {
	case KindBots:
		return "BOTS.md", nil
	case KindMemory:
		return "MEMORY.md", nil
	default:
		return "", ErrInvalidKey
	}
}

func readDocument(key Key, path string) (Document, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Document{Key: key}, nil
	}
	if err != nil {
		return Document{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Document{}, err
	}
	return Document{
		Key:       key,
		Content:   string(data),
		Version:   Digest(data),
		UpdatedAt: info.ModTime().UTC(),
	}, nil
}

// replaceFile writes content beside path and renames it over path atomically.
func replaceFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.WriteString(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
