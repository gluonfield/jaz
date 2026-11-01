package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBotDelegationPromptsAndIdentityRefresh(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	create := func(title, source, owner, parent string) storage.Session {
		t.Helper()
		thread, err := store.CreateSession(storage.CreateSession{
			Title: title, SourceType: source, SourceID: owner, ParentID: parent,
			RuntimeRef: &storage.RuntimeRef{Agent: acp.AgentCodex, SessionID: "native-" + title},
		})
		if err != nil {
			t.Fatal(err)
		}
		return thread
	}
	bot := create("Scout", storage.SourceBot, "", "")
	root := t.TempDir()
	home := t.TempDir()
	bot.RuntimeRef.Cwd = home
	if err := store.SaveSession(bot); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(root, "AGENTS.md"): "Shared personal rules.",
		filepath.Join(root, "BOTS.md"):   "Global bot-only policy.",
		filepath.Join(home, "AGENTS.md"): "This bot coordinates; delegate every engineering task.",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveBot(storage.BotRecord{ThreadID: bot.ID, Kind: bots.KindBot, Prompt: "Follow robotics research."}); err != nil {
		t.Fatal(err)
	}
	group := create("Launch", storage.SourceBot, "", "")
	if err := store.SaveBot(storage.BotRecord{ThreadID: group.ID, Kind: bots.KindGroup, Members: []string{bot.ID}}); err != nil {
		t.Fatal(err)
	}
	seat := create("Group seat", storage.SourceBotMember, bot.ID, "")
	if err := store.SaveMembership(storage.BotMembership{GroupID: group.ID, BotID: bot.ID, ThreadID: seat.ID}); err != nil {
		t.Fatal(err)
	}
	threads := []storage.Session{bot, seat, create("Ordinary", "", "", ""), create("Implementation", storage.SourceBotWorker, bot.ID, seat.ID), create("Routine", storage.SourceLoopRun, "", bot.ID)}
	builder := coordinator.NewBuilder(root, "", "", nil)
	cfg, err := NewACPConfig(Config{}, store, "", builder, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("legacy bot identity remains visible to native workers: %v", err)
	}
	if text, err := os.ReadFile(filepath.Join(root, "AGENTS.md")); err != nil || string(text) != "Shared personal rules." {
		t.Fatalf("migration changed shared personal rules: %q, %v", text, err)
	}
	if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte("Shared workspace instructions."), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewACPConfig(Config{}, store, "", builder, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if text, err := os.ReadFile(filepath.Join(home, "AGENTS.md")); err != nil || string(text) != "Shared workspace instructions." {
		t.Fatalf("repeat startup changed workspace instructions: %q, %v", text, err)
	}
	for _, name := range []string{"Scout", "Reviewer"} {
		bot.Title = name
		if err := store.SaveSession(bot); err != nil {
			t.Fatal(err)
		}
		for _, original := range threads {
			t.Run(name+"/"+original.Title, func(t *testing.T) {
				thread, err := store.LoadSession(original.ID)
				if err != nil {
					t.Fatal(err)
				}
				modules, err := cfg.ResumePrompt(thread)
				if err != nil {
					t.Fatal(err)
				}
				note, version := cfg.TurnNote(thread)
				platform, err := cfg.SystemPrompt.ACPPromptForContext(sessioncontext.WithSourceType(t.Context(), thread.SourceType), "", "chat")
				if err != nil {
					t.Fatal(err)
				}
				prompt := platform + "\n" + modules.Text()
				isBot := thread.SourceType == storage.SourceBot || thread.SourceType == storage.SourceBotMember
				for _, policy := range []string{"Global bot-only policy.", "This bot coordinates; delegate every engineering task.", "All engineering,"} {
					if strings.Contains(prompt, policy) != isBot {
						t.Fatalf("source %q received incorrectly scoped policy %q", thread.SourceType, policy)
					}
				}
				if thread.SourceType != storage.SourceBot && thread.SourceType != storage.SourceBotMember {
					if len(modules) != 0 || note != "" || version != "" {
						t.Fatalf("implementation or ordinary thread received bot instructions: %q, %q, %q", modules, note, version)
					}
					return
				}
				for _, want := range []string{
					"You are " + name + ", a Jaz bot",
					"quick answers, decisions, plans, task and memory coordination",
					"read-only audits of returned reports, diffs, receipts and rendered results",
					"All engineering, including source investigation and diagnosis however brief, goes to implementation threads.",
					"A new problem found during an audit goes back to an implementation thread for investigation.",
					"create_thread for each independent task",
					"continue existing work with send_message_to_thread",
					"Start implementation threads as part of your coordinator role",
					"The thread receives platform instructions and memory and chooses its own approach",
					"Follow robotics research.",
				} {
					if strings.Count(prompt, want) != 1 {
						t.Fatalf("assembled prompt must contain %q once: %q", want, prompt)
					}
				}
				if strings.Contains(prompt, "more than a few quick lookups") || strings.Contains(prompt, "### Tasks") != (thread.SourceType == storage.SourceBot) || strings.Contains(prompt, "A thread inherits your agent") != (thread.SourceType == storage.SourceBot) {
					t.Fatalf("assembled prompt retained the effort exception or changed task/inheritance scope: %q", prompt)
				}
				if !strings.Contains(note, "<bot_instructions>\n"+modules.Text()+"\n</bot_instructions>") {
					t.Fatalf("identity refresh did not deliver the current assembled prompt once: %q", note)
				}
				started := time.Now().UTC()
				if err := store.StartSessionTurn(thread.ID, storage.Turn{StartedAt: started, PromptDigest: version, Prompt: &storage.TurnPrompt{Message: "Review the returned evidence"}}); err != nil {
					t.Fatal(err)
				}
				for _, state := range []storage.PromptState{storage.PromptSending, storage.PromptConfirmed} {
					if err := store.SetTurnPromptState(thread.ID, thread.RuntimeRef.SessionID, started, state); err != nil {
						t.Fatal(err)
					}
				}
				if err := store.FinishSessionTurn(thread.ID, storage.StatusIdle, "", time.Now().UTC(), nil); err != nil {
					t.Fatal(err)
				}
				thread, err = store.LoadSession(thread.ID)
				if err != nil {
					t.Fatal(err)
				}
				if again, held := cfg.TurnNote(thread); again != "" || held != version {
					t.Fatalf("successful delivery repeated the identity or changed its digest: %q, %q", again, held)
				}
			})
		}
		bot, err = store.LoadSession(bot.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
}
