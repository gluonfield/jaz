package instructions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

type fakeLocalInstructionDB struct {
	mu   sync.Mutex
	rows map[string]storage.LocalInstruction
}

func newFakeLocalInstructionDB() *fakeLocalInstructionDB {
	return &fakeLocalInstructionDB{rows: map[string]storage.LocalInstruction{}}
}

func (f *fakeLocalInstructionDB) rowKey(scope, scopeID, kind string) string {
	return scope + "\x00" + scopeID + "\x00" + kind
}

func (f *fakeLocalInstructionDB) GetLocalInstruction(_ context.Context, scope, scopeID, kind string) (storage.LocalInstruction, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.rows[f.rowKey(scope, scopeID, kind)]
	return rec, ok, nil
}

func (f *fakeLocalInstructionDB) ImportLocalInstruction(_ context.Context, doc storage.LocalInstruction) (storage.LocalInstruction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.rowKey(doc.Scope, doc.ScopeID, doc.Kind)
	if existing, ok := f.rows[k]; ok {
		return existing, nil
	}
	f.rows[k] = doc
	return doc, nil
}

func (f *fakeLocalInstructionDB) SaveLocalInstruction(_ context.Context, doc storage.LocalInstruction, expectedVersion string) (storage.LocalInstruction, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.rowKey(doc.Scope, doc.ScopeID, doc.Kind)
	existing, ok := f.rows[k]
	if !ok {
		if expectedVersion != "" {
			return storage.LocalInstruction{Scope: doc.Scope, ScopeID: doc.ScopeID, Kind: doc.Kind}, true, nil
		}
	} else if existing.Version != expectedVersion {
		return existing, true, nil
	}
	f.rows[k] = doc
	return doc, false, nil
}

func TestDBStoreImportsLegacyAgentsOnceAndNeverWritesDisk(t *testing.T) {
	root := t.TempDir()
	agentsPath := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("legacy rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := newFakeLocalInstructionDB()
	fallback := NewLocalFSStore(root)
	store := NewDBStore(db, root, fallback)
	ctx := t.Context()

	agentsKey := Key{Scope: ScopeUser, Kind: KindAgents}
	doc, err := store.Get(ctx, agentsKey)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Content != "legacy rule\n" || doc.Version != Digest([]byte("legacy rule\n")) {
		t.Fatalf("imported doc = %+v", doc)
	}

	// Mutating or removing the legacy file after import must not affect DB reads.
	if err := os.WriteFile(agentsPath, []byte("stale disk edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := store.Get(ctx, agentsKey)
	if err != nil || again.Content != "legacy rule\n" {
		t.Fatalf("second get = %+v, %v", again, err)
	}

	// Saving updates DB only and never modifies AGENTS.md on disk.
	saved, err := store.Save(ctx, agentsKey, "db-only rule\n", doc.Version)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Content != "db-only rule\n" || saved.Version != Digest([]byte("db-only rule\n")) {
		t.Fatalf("saved = %+v", saved)
	}
	rawDisk, err := os.ReadFile(agentsPath)
	if err != nil || string(rawDisk) != "stale disk edit\n" {
		t.Fatalf("DBStore wrote to disk AGENTS.md: %q, %v", rawDisk, err)
	}

	// Clearing AGENTS to empty persists a DB row and must not re-import legacy disk file.
	cleared, err := store.Save(ctx, agentsKey, "", saved.Version)
	if err != nil {
		t.Fatal(err)
	}
	gotCleared, err := store.Get(ctx, agentsKey)
	if err != nil || gotCleared.Content != "" || gotCleared.Version != cleared.Version {
		t.Fatalf("after clear get = %+v, %v", gotCleared, err)
	}

	// Bot files still fall back to LocalFSStore.
	botKey := Key{Scope: ScopeBot, ScopeID: "bot-1", Kind: KindBots}
	if _, err := store.Save(ctx, botKey, "bot rules\n", ""); err != nil {
		t.Fatal(err)
	}
	botDisk, err := os.ReadFile(filepath.Join(root, "bots", "bot-1", "BOTS.md"))
	if err != nil || string(botDisk) != "bot rules\n" {
		t.Fatalf("bot BOTS.md = %q, %v", botDisk, err)
	}
}

func TestDBStoreDoesNotCreateDiskFileOnFreshRootAndEnforcesConcurrency(t *testing.T) {
	root := t.TempDir()
	db := newFakeLocalInstructionDB()
	store := NewDBStore(db, root, NewLocalFSStore(root))
	ctx := t.Context()
	key := Key{Scope: ScopeUser, Kind: KindAgents}

	empty, err := store.Get(ctx, key)
	if err != nil || empty.Content != "" || empty.Version != "" {
		t.Fatalf("fresh get = %+v, %v", empty, err)
	}
	initial, err := store.Save(ctx, key, "v0", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no AGENTS.md on disk, stat err = %v", err)
	}

	const writers = 8
	var okCount, conflictCount atomic.Int32
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
		t.Fatalf("concurrent saves: ok=%d conflict=%d", okCount.Load(), conflictCount.Load())
	}
}
