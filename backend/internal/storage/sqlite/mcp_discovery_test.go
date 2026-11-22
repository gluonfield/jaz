package sqlite

import (
	"reflect"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestDeferredMCPToolsRoundTripPreservesRuntimeAndPolicy(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Runtime: storage.RuntimeACP, RuntimeRef: &storage.RuntimeRef{
		Type: storage.RuntimeACP, Agent: "fake", SessionID: "native-session", MCPServerPolicy: "memory_search_worker",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, deferred := range []*bool{nil, new(true), new(false)} {
		session.RuntimeRef.DeferredMCPTools = deferred
		if err := store.SaveSession(session); err != nil {
			t.Fatal(err)
		}
		loaded, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(loaded.RuntimeRef, session.RuntimeRef) {
			t.Fatalf("runtime changed: got %#v, want %#v", loaded.RuntimeRef, session.RuntimeRef)
		}
	}
}
