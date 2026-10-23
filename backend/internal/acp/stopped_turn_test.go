package acp_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/sessiongoal"
	"github.com/wins/jaz/backend/internal/storage"
)

// promptIndex is where the first prompt containing text came in the agent's
// prompts, or -1.
func (p *jazProcess) promptIndex(text string) int {
	return slices.IndexFunc(p.prompts(), func(prompt string) bool { return strings.Contains(prompt, text) })
}

// Work queued behind a bot's turn that the user stops runs next: the result
// of a thread the bot started and the user's queued message each run once,
// though Jaz restarts while the message's turn works.
func TestWorkQueuedBehindAStoppedTurnRunsOnceAcrossARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	bot := jaz.newBot("Gimli")
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "plan the trip, block until cancelled", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	id, err := jaz.commit(bot.ID, bots.Commit{Request: "Book the hotel", Next: "tell the user"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "hotel", Prompt: "Book the hotel", Directory: t.TempDir(), Commitment: id}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the thread's result queued for the bot", func() bool { return len(jaz.queued(bot.ID)) == 1 })
	jaz.queueUser(bot.ID, `{"text":"and the flights until released"}`)

	jaz.cancel(bot.ID)
	jaz.eventually("the user's message working", func() bool { return jaz.promptsContaining("and the flights") == 1 })
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	jaz = jaz.restart(map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	// The bot reports nothing to the user, so the watchdog wakes it once.
	jaz.settles(bot.ID, 1, 1)
	messages, resumed, results := jaz.promptsContaining("and the flights"), jaz.promptsContaining(continuation), jaz.promptsContaining("completed a turn with state")
	if messages != 1 || resumed != 1 || results != 1 {
		t.Fatalf("the user's message ran %d times, resumed %d times, and the thread's result ran %d times; want each once", messages, resumed, results)
	}
}

// Stopping a goal's turn ends the goal: the thread does not go on with it.
func TestStoppingAGoalsTurnDoesNotContinueTheGoal(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.ID, Message: "draft the plan, block until cancelled", Completion: acp.CompletionAsync, GoalRequested: true}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ID)
	if _, err := sessiongoal.New(store, jaz.events).Create(jaz.ctx, thread.ID, sessiongoal.CreateInput{Objective: "Draft the plan"}); err != nil {
		t.Fatal(err)
	}

	jaz.cancel(thread.ID)
	if session := jaz.session(thread.ID); session.Status != storage.StatusIdle || session.Goal != nil {
		t.Fatalf("stopped thread = %s with goal %+v; want it idle without the goal", session.Status, session.Goal)
	}
	jaz.idle(thread.ID)
	if prompts := jaz.prompts(); len(prompts) != 1 {
		t.Fatalf("prompts = %q; want only the stopped one", prompts)
	}
}

// Steering a queued message into a turn on an agent that cannot steer stops
// the turn and runs that message next; the rest of the queue waits for it.
func TestASteeredMessageRunsBeforeTheQueueBehindIt(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.ID, Message: "plan the trip, block until cancelled", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ID)
	jaz.queueUser(thread.ID, `{"text":"then the hotel"}`)
	jaz.queueUser(thread.ID, `{"text":"the flights first until released"}`)
	res := httptest.NewRecorder()
	jaz.server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/sessions/"+thread.ID+"/queue", strings.NewReader(`{"op":"steer","id":"`+jaz.queued(thread.ID)[1].ID+`"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("steer status = %d, body = %s", res.Code, res.Body.String())
	}

	jaz.eventually("the steered message working", func() bool { return jaz.promptsContaining("the flights first") == 1 })
	session := jaz.session(thread.ID)
	if session.Status != storage.StatusRunning || session.Error != "" || len(session.QueuedMessages) != 1 || jaz.promptsContaining("then the hotel") != 0 {
		t.Fatalf("while the steered message works the thread = %s %q with %d queued; want the rest waiting", session.Status, session.Error, len(session.QueuedMessages))
	}
	jaz.release()
	jaz.idle(thread.ID)
	if steered, rest := jaz.promptIndex("the flights first"), jaz.promptIndex("then the hotel"); rest < steered || jaz.promptsContaining("then the hotel") != 1 {
		t.Fatalf("prompts = %q; want the steered message, then the rest once", jaz.prompts())
	}
}
