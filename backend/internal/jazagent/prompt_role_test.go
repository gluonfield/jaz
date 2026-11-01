package jazagent

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestNativeRequestCarriesItsSessionRole(t *testing.T) {
	for _, source := range []string{"", storage.SourceBotWorker, storage.SourceBot, storage.SourceBotMember} {
		t.Run(source, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			session, err := store.CreateSession(storage.CreateSession{Slug: "role", SourceType: source})
			if err != nil {
				t.Fatal(err)
			}
			turn, err := BuildRequest(t.Context(), store, coordinator.NewBuilder(t.TempDir(), "", "", nil), Request{Session: session, Message: "work"})
			if err != nil {
				t.Fatal(err)
			}
			role := "Session role: work thread."
			if source == storage.SourceBot || source == storage.SourceBotMember {
				role = "Session role: bot chat."
			}
			prompt := provider.MessageContent(turn.Messages[0])
			if strings.Count(prompt, "Session role:") != 1 || !strings.Contains(prompt, role) {
				t.Fatalf("source %q: native request did not receive its role %q", source, role)
			}
		})
	}
}
