package acp_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
)

func (p *jazProcess) namedBot(name string) string {
	p.t.Helper()
	bot, err := p.bots.Create(p.ctx, bots.CreateBot{Name: name})
	if err != nil {
		p.t.Fatal(err)
	}
	return bot.ID
}

// turn waits for thread's agent to work on a turn that hops messages between
// bots led to.
func (p *jazProcess) turn(thread string, hops int) {
	p.t.Helper()
	p.eventually(fmt.Sprintf("a turn of %s after %d hops", thread, hops), func() bool {
		session, err := p.store.LoadSession(thread)
		if err != nil || session.Turn == nil || session.Turn.PendingMessage != nil {
			return false
		}
		output := session.Turn.Output
		return output == nil && hops == 0 || output != nil && output.Hops == hops
	})
	waitForBlockingTool(p.t, p.ctx, p.manager, thread)
}

func (p *jazProcess) cancel(thread string) {
	p.t.Helper()
	if _, err := p.manager.Cancel(p.ctx, thread); err != nil {
		p.t.Fatal(err)
	}
}

func TestBotsWritingToEachOtherStopAtTheHopLimitUntilTheUserWrites(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1", "JAZ_FAKE_ACP_PROMPT_QUEUEING": "1"})
	ann := jaz.namedBot("Ann")
	ben := jaz.namedBot("Ben")
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: ann, Message: "ask Ben and block until cancelled", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	jaz.turn(ann, 0)

	// Ann writes to Ben, Ben answers and each answer has Ann write again.
	for hop := 1; hop <= 6; hop++ {
		if err := jaz.bots.Message(ann, "Ben", "block until cancelled"); err != nil {
			t.Fatalf("message %d: %v", hop, err)
		}
		jaz.cancel(ann)
		jaz.turn(ben, hop)
		if err := jaz.bots.Say(ben, "block until cancelled"); err != nil {
			t.Fatal(err)
		}
		jaz.cancel(ben)
		jaz.turn(ann, hop)
	}
	if err := jaz.bots.Message(ann, "Ben", "block until cancelled"); err == nil {
		t.Fatal("the bots wrote to each other a seventh time without the user")
	}

	if _, _, err := acp.NewMCPTools(jaz.manager).Send(jaz.ctx, callAs(ben), acp.MCPSendInput{ThreadID: ann, Prompt: "Use the correction and block until cancelled"}); err != nil {
		t.Fatal(err)
	}
	if err := jaz.bots.Message(ann, "Ben", "block until cancelled"); err == nil {
		t.Fatal("a bot follow-up reset the exchange limit")
	}
	if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: ann, Message: "ask Ben once more and block until cancelled", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	if err := jaz.bots.Message(ann, "Ben", "block until cancelled"); err != nil {
		t.Fatalf("after the user wrote: %v", err)
	}
}

func TestARoutineRunInItsBotsThreadRecordsWhetherItDelivered(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.namedBot("Gimli")
	quiet := jaz.routine(loops.CreateLoop{Name: "Notes", Prompt: "check the notes", BotID: bot, Thread: loops.ThreadMain})
	run, err := jaz.loops.RunNow(jaz.ctx, quiet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if jaz.waitRun(run.ID, loops.RunStatusOK).Delivered {
		t.Fatal("a run that sent nothing delivered")
	}

	talking := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox until released", BotID: bot, Thread: loops.ThreadMain})
	if run, err = jaz.loops.RunNow(jaz.ctx, talking.ID); err != nil {
		t.Fatal(err)
	}
	jaz.waitRun(run.ID, loops.RunStatusRunning)
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot)
	if err := jaz.bots.Say(bot, "Two new emails."); err != nil {
		t.Fatal(err)
	}
	jaz.release()
	if !jaz.waitRun(run.ID, loops.RunStatusOK).Delivered {
		t.Fatal("a run that sent its outcome did not deliver")
	}
	res := httptest.NewRecorder()
	jaz.server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/loops/"+talking.ID+"/runs", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"delivered":true`) {
		t.Fatalf("runs = %d %s", res.Code, res.Body.String())
	}
}

func TestARoutinesThreadWritesToOtherBotsAsItsBotAndAThreadItStartedCannot(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	gimli := jaz.namedBot("Gimli")
	jaz.namedBot("Legolas")
	routine := jaz.routine(loops.CreateLoop{Name: "Scout", Prompt: "scout until released", BotID: gimli})
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	child := jaz.waitRun(run.ID, loops.RunStatusRunning).ThreadID
	worker, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "errand", ParentID: gimli, Commitment: owe(t, store, gimli)})
	if err != nil {
		t.Fatal(err)
	}

	if err := jaz.bots.Message(child, "Legolas", "Orcs to the east."); err != nil {
		t.Fatal(err)
	}
	if err := jaz.bots.Message(child, "Gimli", "Orcs to the east."); err == nil {
		t.Fatal("a thread of Gimli's wrote to Gimli as another bot")
	}
	// Gimli's own threads report to Gimli, which speaks for them.
	if err := jaz.bots.Message(worker.ID, "Legolas", "Orcs to the west."); err == nil || !strings.Contains(err.Error(), "cannot message bots") {
		t.Fatalf("a thread Gimli started wrote to another bot: %v", err)
	}
	jaz.release()
	jaz.eventually("the run's message reaching Legolas", func() bool { return jaz.promptsContaining("[message from Gimli]") == 1 })
	if got := jaz.promptsContaining("[message from Jaz]"); got != 0 {
		t.Fatalf("%d messages signed as Jaz", got)
	}
}
