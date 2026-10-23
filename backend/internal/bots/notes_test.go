package bots

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestABotIsShownTheTasksThatChangedSinceItsAgentLastSawThem(t *testing.T) {
	service, store, _ := newHomeService(t)
	bot, err := service.Create(t.Context(), CreateBot{Name: "Research", Prompt: "Follow robotics research."})
	if err != nil {
		t.Fatal(err)
	}
	open := func(request, next string) storage.Commitment {
		t.Helper()
		commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: request, Note: next})
		if err != nil {
			t.Fatal(err)
		}
		return commitment
	}
	quotes, brochure := open("Compare the quotes", "send the comparison"), open("Draft the brochure", "ask for the logo")
	thread, err := store.LoadSession(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	thread.RuntimeRef.Agent, thread.RuntimeRef.SessionID = acp.AgentCodex, "first"
	thread.Usage.ContextTokens = 1000
	// A turn that succeeds keeps what its agent was shown.
	turn := func() string {
		t.Helper()
		note, recorded := TurnNote(store)(thread)
		thread.PromptDigest = recorded
		return note
	}
	quotesLine := fmt.Sprintf("#%d [pending] for the user: Compare the quotes · next: send the comparison · nothing moves on it", quotes.ID)
	everything := func(note string) bool {
		return strings.Contains(note, quotesLine) && strings.Contains(note, "Draft the brochure") && !strings.Contains(note, "Unchanged")
	}

	if note := turn(); !everything(note) || !strings.Contains(note, "<bot_instructions>") {
		t.Fatalf("the first turn opened with %q, want the bot's identity and every open task", note)
	}
	thread.Usage.ContextTokens = 2000
	if note := turn(); note != "" {
		t.Fatalf("a turn after nothing changed opened with %q, want nothing", note)
	}
	brochure.Note = "send the draft"
	if err := store.UpdateCommitment(brochure); err != nil {
		t.Fatal(err)
	}
	if note := turn(); !strings.Contains(note, "Draft the brochure · next: send the draft") || strings.Contains(note, "Compare the quotes") || !strings.Contains(note, fmt.Sprintf("Unchanged: #%d.", quotes.ID)) {
		t.Fatalf("after one task moved the turn opened with %q, want only it and the other's number", note)
	}
	// A message steering a turn is shown what changed since the turn started.
	quotes.Note = "send the cheaper one"
	if err := store.UpdateCommitment(quotes); err != nil {
		t.Fatal(err)
	}
	note, started := TurnNote(store)(thread)
	thread.Turn = &storage.Turn{PromptDigest: started}
	if steer := SteerNote(store)(thread); !strings.Contains(note, "send the cheaper one") || steer != "" {
		t.Fatalf("the turn opened with %q and its steer with %q, want the moved task once", note, steer)
	}
	thread.Turn, thread.PromptDigest = nil, started
	quotesLine = strings.Replace(quotesLine, "send the comparison", "send the cheaper one", 1)
	// A compaction shrinks what the agent holds, and a new session holds none.
	thread.Usage.ContextTokens = 500
	if note := turn(); !everything(note) {
		t.Fatalf("after a compaction the turn opened with %q, want every open task", note)
	}
	thread.RuntimeRef.SessionID = "second"
	if note := turn(); !everything(note) {
		t.Fatalf("in a new session the turn opened with %q, want every open task", note)
	}
	// A thread last shown its identity before tasks were kept is not shown it again.
	thread.PromptDigest = lastShown(thread.PromptDigest).Identity
	if note := turn(); strings.Contains(note, "<bot_instructions>") || !everything(note) {
		t.Fatalf("after the upgrade the turn opened with %q, want every open task and no identity", note)
	}
	// Lines shown before a session has an id are shown again once it has one.
	thread.RuntimeRef.SessionID = ""
	turn()
	thread.RuntimeRef.SessionID = "third"
	if note := turn(); !everything(note) {
		t.Fatalf("once the session had an id the turn opened with %q, want every open task", note)
	}
	if err := store.DropCommitment(brochure.ID, "not needed", time.Now()); err != nil {
		t.Fatal(err)
	}
	if note := turn(); note != fmt.Sprintf("No longer open: #%d.\n\n", brochure.ID) {
		t.Fatalf("after a task closed the turn opened with %q, want only its number", note)
	}
	if note := turn(); note != "" {
		t.Fatalf("a turn after the closed task was shown opened with %q, want nothing", note)
	}
}
