package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestRuntimeSessionMaterializationState(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug: "materialization",
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: "codex",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasTranscript, err := store.HasSessionTranscript(session.ID); err != nil || hasTranscript {
		t.Fatalf("empty transcript = %t, %v", hasTranscript, err)
	}
	if updated, err := store.ReplaceRuntimeSessionID(session.ID, "wrong", "new"); err != nil || updated {
		t.Fatalf("mismatched replacement = %t, %v", updated, err)
	}
	if updated, err := store.ReplaceRuntimeSessionID(session.ID, "", "new"); err != nil || !updated {
		t.Fatalf("matched replacement = %t, %v", updated, err)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RuntimeRef.SessionID != "new" {
		t.Fatalf("runtime session = %q", loaded.RuntimeRef.SessionID)
	}
	if err := storage.AppendUserMessage(store, session.ID, "started", nil, nil); err != nil {
		t.Fatal(err)
	}
	if hasTranscript, err := store.HasSessionTranscript(session.ID); err != nil || !hasTranscript {
		t.Fatalf("started transcript = %t, %v", hasTranscript, err)
	}
}
