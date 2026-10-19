package jazagent

import (
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestNativeRequestRequiresExplicitHumanAuthorship(t *testing.T) {
	for _, human := range []bool{false, true} {
		store, err := sqlitestore.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		session, err := store.CreateSession(storage.CreateSession{Slug: "native-authorship"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := BuildRequest(t.Context(), store, nil, Request{Session: session, Message: "continue", AppendUser: true, FromUser: human}); err != nil {
			t.Fatal(err)
		}
		last, err := store.LastUserMessageAt()
		if err != nil || last.IsZero() == human {
			t.Fatalf("human=%t: last activity = %v, %v", human, last, err)
		}
	}
}
