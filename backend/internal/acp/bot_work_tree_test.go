package acp_test

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

// A thread a bot's subtask starts is the bot's work, titled like any thread,
// and reports to the subtask that started it, never to the bot.
func TestAThreadASubtaskStartsIsTheBotsWorkAndReportsToTheSubtask(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	// As serve.go wires it.
	jaz.manager.ThreadStarted = jaz.server.TitleThread
	bot := jaz.newBot("Gimli")
	tools := acp.NewMCPTools(jaz.manager)
	_, worker, err := tools.Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "brochure", Title: "Brochure", Prompt: "Draft the brochure until released", Commitment: owe(t, store, bot.ID)})
	if err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, worker.ThreadID)
	_, review, err := tools.Create(jaz.ctx, callAs(worker.ThreadID), acp.MCPCreateInput{Slug: "draft-review", Prompt: "Review the draft"})
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the review's result reaching the subtask that started it", func() bool {
		return slices.ContainsFunc(jaz.queued(worker.ThreadID), func(message storage.QueuedMessage) bool { return message.From == review.ThreadID })
	})
	if slices.ContainsFunc(jaz.queued(bot.ID), func(message storage.QueuedMessage) bool { return message.From == review.ThreadID }) {
		t.Fatal("the review's result went to the bot")
	}
	stored, err := store.LoadSession(review.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SourceType != storage.SourceBotWorker || stored.SourceID != bot.ID || stored.ParentID != worker.ThreadID {
		t.Fatalf("review = %s/%s under %s, want the bot's work under the subtask", stored.SourceType, stored.SourceID, stored.ParentID)
	}
	jaz.eventually("the review titled from its prompt", func() bool {
		stored, err := store.LoadSession(review.ThreadID)
		return err == nil && stored.TitleLocked && stored.Title == "hello from fake agent"
	})
	if stored, err := store.LoadSession(worker.ThreadID); err != nil || stored.Title != "Brochure" {
		t.Fatalf("the subtask's own title became %q, %v", stored.Title, err)
	}
	jaz.release()
	jaz.eventually("the subtask reading the review's result", func() bool {
		return jaz.promptsContaining("ACP session draft-review") == 1
	})
}
