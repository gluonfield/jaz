package acp_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

// queueUser queues message, as JSON, in thread the way the composer does.
func (p *jazProcess) queueUser(thread, message string) {
	p.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+thread+"/queue", strings.NewReader(`{"op":"append","message":`+message+`}`))
	res := httptest.NewRecorder()
	p.server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		p.t.Fatalf("queue status = %d, body = %s", res.Code, res.Body.String())
	}
}

// compacts counts the provider compacts sent to the agent.
func (p *jazProcess) compacts() int {
	return p.promptsContaining(acp.CompactCommand)
}

func TestAFullBotCompactsOnceWhenATurnLeavesItIdle(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_HOLD_TEXT": acp.CompactCommand})
	bot := jaz.newBot("Gimli")

	jaz.write(bot.ID, "offer compact, plan the week context 790/1000")
	if got := jaz.compacts(); got != 0 {
		t.Fatalf("compacts = %d below the threshold, want none", got)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "draft the reply context 800/1000", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	// The user reads the reply while the thread compacts.
	jaz.eventually("the compact working", func() bool { return jaz.compacts() == 1 })
	res := httptest.NewRecorder()
	jaz.server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/seen", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("seen status = %d, body = %s", res.Code, res.Body.String())
	}
	jaz.release()
	// The compact turn reports no new fill, so the thread still reads full
	// after it; only an ordinary turn's end may queue a compact.
	jaz.idle(bot.ID)
	if got := jaz.compacts(); got != 1 {
		t.Fatalf("compacts = %d, want one after the turn that filled the context", got)
	}
	if prompts := jaz.prompts(); !strings.Contains(prompts[len(prompts)-1], acp.CompactCommand) {
		t.Fatalf("prompts = %q, want the compact right after the full turn", prompts)
	}
	if session, err := store.LoadSession(bot.ID); err != nil || session.Unread {
		t.Fatalf("bot = %+v, %v; want the read reply to stay read after the compact", session, err)
	}
	if bots, err := jaz.bots.List(); err != nil || len(bots) != 1 || bots[0].ContextTokens != 800 || bots[0].ContextWindowTokens != 1000 {
		t.Fatalf("bots = %+v, %v; want the bot's fill", bots, err)
	}
}

func TestNoCompactWithUnknownFillOrCommandOutsideABotOrWithWorkQueued(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	jaz.write(jaz.newBot("Gimli").ID, "offer compact, plan the week context 990/0")
	jaz.write(jaz.newBot("Gandalf").ID, "withdraw compact context 990/1000")
	jaz.write(jaz.newBot("Aragorn").ID, "plan the week context 990/1000")
	chat, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	jaz.write(chat.ID, "offer compact, plan the week context 990/1000")
	if got := jaz.compacts(); got != 0 {
		t.Fatalf("compacts = %d, want none for an unknown window, an agent that did not offer compact or a chat that is not a bot", got)
	}

	bot := jaz.newBot("Legolas")
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "offer compact, work context 900/1000 until released", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	jaz.queueUser(bot.ID, `{"text":"and then this context 950/1000"}`)
	jaz.release()
	jaz.idle(bot.ID)
	prompts := jaz.prompts()
	followUp := slices.IndexFunc(prompts, func(prompt string) bool { return strings.Contains(prompt, "and then this") })
	compact := slices.IndexFunc(prompts, func(prompt string) bool { return strings.Contains(prompt, acp.CompactCommand) })
	if followUp < 0 || compact != followUp+1 || jaz.compacts() != 1 {
		t.Fatalf("prompts = %q, want one compact, after the queued message", prompts)
	}
}

func TestAQueuedCompactThatCannotStartDoesNotHoldUpTheUser(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "withdraw compact until released", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	jaz.queueUser(bot.ID, `{"action":"compact"}`)
	jaz.queueUser(bot.ID, `{"text":"are you there"}`)

	jaz.release()
	jaz.idle(bot.ID)
	if jaz.promptsContaining("are you there") != 1 || jaz.compacts() != 0 {
		t.Fatalf("prompts = %q, want the user's message run after the compact failed", jaz.prompts())
	}
}
