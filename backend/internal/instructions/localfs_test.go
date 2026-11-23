package instructions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDigestMatches16HexSHA256Prefix(t *testing.T) {
	input := []byte("standing rule\n")
	sum := sha256.Sum256(input)
	want := hex.EncodeToString(sum[:8])
	if got := Digest(input); got != want || len(got) != 16 {
		t.Fatalf("Digest = %q (len %d), want %q", got, len(got), want)
	}
}

func TestLocalFSStoreUserAndBotDocuments(t *testing.T) {
	root := t.TempDir()
	store := NewLocalFSStore(root)
	ctx := t.Context()

	agentsKey := Key{Scope: ScopeUser, Kind: KindAgents}
	doc, err := store.Get(ctx, agentsKey)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Content != "" || doc.Version != "" || !doc.UpdatedAt.IsZero() {
		t.Fatalf("missing doc = %+v, want empty version and content", doc)
	}

	saved, err := store.Save(ctx, agentsKey, "Always cite sources.\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Content != "Always cite sources.\n" || saved.Version != Digest([]byte("Always cite sources.\n")) || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved doc = %+v", saved)
	}
	raw, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil || string(raw) != "Always cite sources.\n" {
		t.Fatalf("disk AGENTS.md = %q, %v", raw, err)
	}

	current, err := store.Save(ctx, agentsKey, "overwrite\n", "")
	if !errors.Is(err, ErrConflict) || current.Content != "Always cite sources.\n" || current.Version != saved.Version {
		t.Fatalf("stale save = (%+v, %v), want ErrConflict with current doc", current, err)
	}

	// Same-length rewrite with identical mtime must still conflict via content digest.
	info, err := os.Stat(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Always cite papers!.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "AGENTS.md"), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(ctx, agentsKey, "my edit\n", saved.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("same-mtime rewrite err = %v, want ErrConflict", err)
	}

	botKey := Key{Scope: ScopeBot, ScopeID: "bot-1", Kind: KindMemory}
	botDoc, err := store.Save(ctx, botKey, "Remember Biscuit.\n", "")
	if err != nil {
		t.Fatal(err)
	}
	gotBot, err := store.Get(ctx, botKey)
	if err != nil || gotBot.Content != "Remember Biscuit.\n" || gotBot.Version != botDoc.Version {
		t.Fatalf("bot memory get = (%+v, %v)", gotBot, err)
	}
}

func TestLocalFSStoreCustomBotHomeResolverAndValidation(t *testing.T) {
	customHome := t.TempDir()
	store := NewLocalFSStore(t.TempDir(), WithBotHomeResolver(func(botID string) (string, error) {
		if botID == "custom" {
			return customHome, nil
		}
		return "", errors.New("unknown bot")
	}))
	ctx := t.Context()

	if _, err := store.Save(ctx, Key{Scope: ScopeBot, ScopeID: "custom", Kind: KindBots}, "Coordinate.\n", ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(customHome, "BOTS.md"))
	if err != nil || string(data) != "Coordinate.\n" {
		t.Fatalf("custom home BOTS.md = %q, %v", data, err)
	}

	unsupportedKeys := []Key{
		{Scope: ScopeOrg, ScopeID: "org-1", Kind: KindAgents},
		{Scope: ScopeSpace, ScopeID: "space-1", Kind: KindAgents},
	}
	for _, key := range unsupportedKeys {
		if err := key.Validate(); err != nil {
			t.Errorf("Validate(%+v) err = %v, want nil", key, err)
		}
		if _, err := store.Get(ctx, key); !errors.Is(err, ErrUnsupportedScope) {
			t.Errorf("Get(%+v) err = %v, want ErrUnsupportedScope", key, err)
		}
		if _, err := store.Save(ctx, key, "x", ""); !errors.Is(err, ErrUnsupportedScope) {
			t.Errorf("Save(%+v) err = %v, want ErrUnsupportedScope", key, err)
		}
	}

	invalidKeys := []Key{
		{Scope: ScopeSpace, ScopeID: "", Kind: KindAgents},
		{Scope: ScopeUser, Kind: KindMemory},
		{Scope: ScopeBot, ScopeID: "", Kind: KindBots},
		{Scope: ScopeBot, ScopeID: "custom", Kind: KindAgents},
		{Scope: ScopeUser, Kind: Kind("soul")},
		{Scope: Scope("unknown"), Kind: KindAgents},
	}
	for _, key := range invalidKeys {
		if _, err := store.Get(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Get(%+v) err = %v, want ErrInvalidKey", key, err)
		}
		if _, err := store.Save(ctx, key, "x", ""); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Save(%+v) err = %v, want ErrInvalidKey", key, err)
		}
	}

	plain := NewLocalFSStore(t.TempDir())
	for _, badBot := range []string{"../escape", "a/b", `a\b`, ".."} {
		if _, err := plain.Get(ctx, Key{Scope: ScopeBot, ScopeID: badBot, Kind: KindBots}); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("bot traversal %q err = %v, want ErrInvalidKey", badBot, err)
		}
	}
}

func TestLocalFSStoreConcurrentOptimisticSave(t *testing.T) {
	store := NewLocalFSStore(t.TempDir())
	key := Key{Scope: ScopeUser, Kind: KindAgents}
	ctx := t.Context()

	initial, err := store.Save(ctx, key, "v0", "")
	if err != nil {
		t.Fatal(err)
	}

	const writers = 8
	var okCount atomic.Int32
	var conflictCount atomic.Int32
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, saveErr := store.Save(ctx, key, "v1-"+string(rune('a'+id)), initial.Version)
			switch {
			case saveErr == nil:
				okCount.Add(1)
			case errors.Is(saveErr, ErrConflict):
				conflictCount.Add(1)
			default:
				t.Errorf("unexpected error: %v", saveErr)
			}
		}(i)
	}
	wg.Wait()
	if okCount.Load() != 1 || conflictCount.Load() != writers-1 {
		t.Fatalf("concurrent saves: ok=%d conflict=%d, want 1 and %d", okCount.Load(), conflictCount.Load(), writers-1)
	}
}
