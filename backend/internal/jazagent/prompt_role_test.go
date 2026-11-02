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
			role := "Session role: work thread."
			if source == storage.SourceBot || source == storage.SourceBotMember {
				role = "Session role: bot chat."
			}
			approval := "I approve committing, pushing and releasing this verified correction."
			for _, phase := range []string{"fresh", "resumed"} {
				if phase == "resumed" {
					if err := store.SaveMessages(session.ID, []provider.Message{provider.SystemMessage("obsolete system prompt"), provider.UserMessage(approval)}); err != nil {
						t.Fatal(err)
					}
				}
				turn, err := BuildRequest(t.Context(), store, coordinator.NewBuilder(root, "", "", nil), Request{Session: session, Message: "work"})
				if err != nil {
					t.Fatal(err)
				}
				prompt := provider.MessageContent(turn.Messages[0])
				if strings.Count(prompt, "Session role:") != 1 || !strings.Contains(prompt, role) {
					t.Fatalf("source %q: %s native request did not receive its role %q", source, phase, role)
				}
				for _, rule := range []string{
					"Never take an irreversible action, including sending email or messages, without the user's explicit consent for that specific action.",
					"the exact message content, recipients and attachments. Wait for approval before acting.",
					"Research, analysis, drafting, access to data and general task instructions are not consent.",
					"Prior explicit authorization remains valid only within its approved scope; do not ask again while it still applies.",
					"Changes to the action, message content, recipients or attachments require fresh approval.",
				} {
					if strings.Count(prompt, rule) != 1 {
						t.Fatalf("source %q: %s native request must carry consent rule once: %q", source, phase, rule)
					}
				}
				isBot := source == storage.SourceBot || source == storage.SourceBotMember
				for _, rule := range []string{"quick coordination plans", "substantive engineering planning and substantive product design", "Delegate that work even when the requested deliverable is a plan, visual or prototype.", "Artifact guidance does not move that work into this chat."} {
					if strings.Contains(prompt, rule) != isBot {
						t.Fatalf("source %q: %s native request has incorrectly scoped planning policy %q", source, phase, rule)
					}
				}
				if strings.Contains(prompt, "Only bots delegate engineering.") != isBot || strings.Contains(prompt, "All engineering,") != isBot || strings.Contains(prompt, "Launching background work is not delivery") == isBot {
					t.Fatalf("source %q received contradictory role instructions: %s", source, prompt)
				}
				if phase == "resumed" && (len(turn.Messages) != 3 || provider.MessageContent(turn.Messages[1]) != approval) {
					t.Fatal("resumed request altered prior authorization or retained the obsolete prompt")
				}
				t.Logf("%s %q: role and consent once; %d system-prompt bytes; prior authorization preserved", phase, source, len(prompt))
			}
		})
	}
}
