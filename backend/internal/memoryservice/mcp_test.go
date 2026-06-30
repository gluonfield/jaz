package memoryservice

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestGatedJazmemGetPageAcceptsAbsoluteMemoryPage(t *testing.T) {
	root := t.TempDir()
	mem, err := jazmem.Open(jazmem.Config{Root: root, DBPath: filepath.Join(t.TempDir(), "index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mem.Close() })
	if err := os.MkdirAll(filepath.Join(root, "sources", "chat", "telegram", "42"), 0o755); err != nil {
		t.Fatal(err)
	}
	pagePath := filepath.Join(root, "sources", "chat", "telegram", "42", "contacts.md")
	if err := os.WriteFile(pagePath, []byte("# Contacts\n\n- Alice"), 0o644); err != nil {
		t.Fatal(err)
	}

	page, err := (gatedJazmem{service: New(mem, memorySettingsStore{}, nil, "")}).GetPage(context.Background(), pagePath)
	if err != nil {
		t.Fatal(err)
	}
	if page.Slug != "sources/chat/telegram/42/contacts" || page.Body != "# Contacts\n\n- Alice" {
		t.Fatalf("page = %#v", page)
	}
}

func TestGatedJazmemGetPageRejectsAbsolutePathOutsideMemoryRoot(t *testing.T) {
	root := t.TempDir()
	mem, err := jazmem.Open(jazmem.Config{Root: root, DBPath: filepath.Join(t.TempDir(), "index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mem.Close() })

	_, err = (gatedJazmem{service: New(mem, memorySettingsStore{}, nil, "")}).GetPage(context.Background(), filepath.Join(t.TempDir(), "raw.jsonl"))
	if err == nil {
		t.Fatal("expected absolute non-memory path to be rejected")
	}
}

func TestNormalizeMemoryPagePath(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"slug", "people/alice", "people/alice"},
		{"relative markdown", "sources/chat/telegram/42/contacts.md", "sources/chat/telegram/42/contacts"},
		{"absolute markdown", filepath.Join(root, "sources", "chat", "telegram", "42", "contacts.md"), "sources/chat/telegram/42/contacts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeMemoryPagePath(root, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("normalizeMemoryPagePath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

type memorySettingsStore struct{}

func (memorySettingsStore) LoadSetting(_, _ string) (storage.Setting, error) {
	return storage.Setting{}, storage.ErrSettingNotFound
}

func (memorySettingsStore) SaveSetting(namespace, key string, value json.RawMessage) (storage.Setting, error) {
	return storage.Setting{Namespace: namespace, Key: key, Value: value}, nil
}

func (memorySettingsStore) DeleteSetting(_, _ string) error { return nil }

func (memorySettingsStore) ListSettings(string) ([]storage.Setting, error) { return nil, nil }
