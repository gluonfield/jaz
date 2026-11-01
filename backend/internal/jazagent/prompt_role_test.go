package jazagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestNativeRequestCarriesItsSessionRole(t *testing.T) {
	for _, source := range []string{"", storage.SourceBotWorker, storage.SourceBot, storage.SourceBotMember, storage.SourceLoopRun} {
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
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "BOTS.md"), []byte("Only bots delegate engineering."), 0o600); err != nil {
				t.Fatal(err)
			}
			turn, err := BuildRequest(t.Context(), store, coordinator.NewBuilder(root, "", "", nil), Request{Session: session, Message: "work"})
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
			isBot := source == storage.SourceBot || source == storage.SourceBotMember
			if strings.Contains(prompt, "Only bots delegate engineering.") != isBot || strings.Contains(prompt, "All engineering,") != isBot || strings.Contains(prompt, "Launching background work is not delivery") == isBot {
				t.Fatalf("source %q received contradictory role instructions: %s", source, prompt)
			}
		})
	}
}
