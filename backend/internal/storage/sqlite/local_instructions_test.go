package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestLocalInstructionsStorePersistenceAndOptimisticConcurrency(t *testing.T) {
	ctx := context.Background()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, found, err := store.GetLocalInstruction(ctx, "user", "", "agents"); err != nil || found {
		t.Fatalf("expected absent row, found=%v err=%v", found, err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	// Stale expectedVersion on missing row must conflict.
	if _, conflict, err := store.SaveLocalInstruction(ctx, storage.LocalInstruction{
		Scope: "user", ScopeID: "", Kind: "agents", BodyMD: "v1", Version: "ver1", UpdatedAt: now,
	}, "stale"); err != nil || !conflict {
		t.Fatalf("expected conflict for stale version on missing row, conflict=%v err=%v", conflict, err)
	}

	imported, err := store.ImportLocalInstruction(ctx, storage.LocalInstruction{
		Scope: "user", ScopeID: "", Kind: "agents", BodyMD: "imported", Version: "ver0", UpdatedAt: now,
	})
	if err != nil || imported.BodyMD != "imported" {
		t.Fatalf("ImportLocalInstruction = %+v, %v", imported, err)
	}
	// Second import must preserve the existing row.
	secondImport, err := store.ImportLocalInstruction(ctx, storage.LocalInstruction{
		Scope: "user", ScopeID: "", Kind: "agents", BodyMD: "overwrite", Version: "verX", UpdatedAt: now,
	})
	if err != nil || secondImport.BodyMD != "imported" || secondImport.Version != "ver0" {
		t.Fatalf("second ImportLocalInstruction = %+v, %v", secondImport, err)
	}

	saved, conflict, err := store.SaveLocalInstruction(ctx, storage.LocalInstruction{
		Scope: "user", ScopeID: "", Kind: "agents", BodyMD: "updated", Version: "ver1", UpdatedAt: now.Add(time.Second),
	}, "ver0")
	if err != nil || conflict || saved.BodyMD != "updated" {
		t.Fatalf("SaveLocalInstruction = %+v conflict=%v err=%v", saved, conflict, err)
	}

	cur, conflict, err := store.SaveLocalInstruction(ctx, storage.LocalInstruction{
		Scope: "user", ScopeID: "", Kind: "agents", BodyMD: "stale", Version: "ver2", UpdatedAt: now.Add(2 * time.Second),
	}, "ver0")
	if err != nil || !conflict || cur.BodyMD != "updated" || cur.Version != "ver1" {
		t.Fatalf("stale SaveLocalInstruction = %+v conflict=%v err=%v", cur, conflict, err)
	}

	got, found, err := store.GetLocalInstruction(ctx, "user", "", "agents")
	if err != nil || !found || got.BodyMD != "updated" || got.Version != "ver1" {
		t.Fatalf("GetLocalInstruction = %+v found=%v err=%v", got, found, err)
	}
}
