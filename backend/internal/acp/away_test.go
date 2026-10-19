package acp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/httpapi/agentsessions"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

var hourly = loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}

func (p *jazProcess) newBot(name string) bots.Bot {
	p.t.Helper()
	bot, err := p.bots.Create(p.ctx, bots.CreateBot{Name: name, Agent: acp.AgentCodex})
	if err != nil {
		p.t.Fatal(err)
	}
	return bot
}

// write sends the user's message to thread and waits for its turn to end.
func (p *jazProcess) write(thread, text string) {
	p.t.Helper()
	if _, err := p.manager.Send(p.ctx, acp.SendRequest{Session: thread, Message: text, Completion: acp.CompletionAsync, FromUser: true}); err != nil {
		p.t.Fatal(err)
	}
	p.idle(thread)
}

func (p *jazProcess) idle(thread string) {
	p.t.Helper()
	p.eventually("the turn in "+thread+" ending", func() bool {
		session, err := p.store.LoadSession(thread)
		return err == nil && session.Status == storage.StatusIdle && len(session.QueuedMessages) == 0
	})
}

// notices lists what bot posted in its own chat.
func (p *jazProcess) notices(bot string) []string {
	p.t.Helper()
	events, err := p.store.LoadSessionEvents(bot)
	if err != nil {
		p.t.Fatal(err)
	}
	var notices []string
	for _, event := range events {
		if event.RoomMessage != nil && event.RoomMessage.BotID == bot {
			notices = append(notices, event.RoomMessage.Text)
		}
	}
	return notices
}

// guard runs the away guard at a moment of the test's choosing.
func (p *jazProcess) guard(at time.Time) {
	p.t.Helper()
	p.loops.Now = func() time.Time { return at }
	if err := p.loops.GuardAway(); err != nil {
		p.t.Fatal(err)
	}
}

func (p *jazProcess) dueAt(at time.Time) {
	p.t.Helper()
	p.loops.Now = func() time.Time { return at }
	if _, err := p.loops.StartDueAll(p.ctx); err != nil {
		p.t.Fatal(err)
	}
}

func TestThreeDaysAwayHoldBotRoutinesButKeepRunningOnesAcrossARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	inbox := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly})
	monitor := jaz.routine(loops.CreateLoop{Name: "Monitor", Prompt: "watch the furnace", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly, KeepRunning: true})
	quotes := jaz.routine(loops.CreateLoop{Name: "Quotes", Prompt: "triage the quote", BotID: bot.ID, Thread: loops.ThreadMain, Trigger: quotes})
	hook := jaz.routine(loops.CreateLoop{Name: "Hook", Prompt: "handle the hook", BotID: bot.ID, Thread: loops.ThreadMain, Trigger: &loops.Trigger{Kind: loops.TriggerWebhook}})
	jaz.write(bot.ID, "hello")
	wrote := time.Now()

	jaz.guard(wrote.Add(72*time.Hour - time.Minute))
	if notices := jaz.notices(bot.ID); len(notices) != 0 {
		t.Fatalf("notices = %q a minute before three days, want none", notices)
	}
	slot := wrote.Add(73 * time.Hour).Truncate(time.Hour)
	jaz.guard(slot.Add(time.Second))
	notices := jaz.notices(bot.ID)
	if len(notices) != 1 || !strings.Contains(notices[0], "Inbox") || strings.Contains(notices[0], "Monitor") {
		t.Fatalf("notices = %q, want one naming the routine that waits", notices)
	}
	jaz.dueAt(slot.Add(time.Second))
	if run := jaz.runs(inbox.ID)[0]; run.Status != loops.RunStatusSkipped || !strings.Contains(run.Error, "3 days") {
		t.Fatalf("inbox run = %+v, want it skipped while the user is away", run)
	}
	jaz.waitRun(jaz.runs(monitor.ID)[0].ID, loops.RunStatusOK)
	jaz.loops.HandleIncoming(jaz.ctx, quote("m1", "A quote", slot))
	if _, err := jaz.loops.RunTriggered(jaz.ctx, hook.ID, "webhook"); err != nil {
		t.Fatal(err)
	}
	if runs := append(jaz.runs(quotes.ID), jaz.runs(hook.ID)...); len(runs) != 0 {
		t.Fatalf("event runs = %+v, want none while the user is away", runs)
	}

	jaz = jaz.restart(nil)
	jaz.guard(slot.Add(time.Hour + time.Second))
	jaz.dueAt(slot.Add(time.Hour + time.Second))
	if run := jaz.runs(inbox.ID)[0]; run.Status != loops.RunStatusSkipped {
		t.Fatalf("inbox run after a restart = %+v, want it still waiting", run)
	}
	jaz.waitRun(jaz.runs(monitor.ID)[0].ID, loops.RunStatusOK)
	if notices := jaz.notices(bot.ID); len(notices) != 1 {
		t.Fatalf("notices after a restart = %q, want the one already sent", notices)
	}
	if routine, err := jaz.loops.Load(inbox.ID); err != nil || routine.Status != loops.StatusActive {
		t.Fatalf("inbox = %+v, %v; want its own status untouched", routine, err)
	}
	items, err := jaz.loops.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		detail, err := jaz.loops.Load(item.ID)
		if err != nil || detail.Blocked != item.Blocked {
			t.Fatalf("list/detail waiting reason differs: %+v, %+v, %v", item, detail, err)
		}
		if (item.Blocked != "") != (item.ID == inbox.ID) {
			t.Fatalf("routine %s blocked = %q, want only the away-paused schedule", item.Name, item.Blocked)
		}
	}
	jaz.write(bot.ID, "back now")
	jaz.guard(time.Now())
	if routine, err := jaz.loops.Load(inbox.ID); err != nil || routine.Blocked != "" {
		t.Fatalf("inbox after return = %+v, %v", routine, err)
	}
}

func TestOnlyTheUsersNextWordsLetRoutinesRunAgain(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	bot := jaz.newBot("Gimli")
	group, err := jaz.bots.CreateGroup("Crew", []string{bot.ID, jaz.newBot("Legolas").ID})
	if err != nil {
		t.Fatal(err)
	}
	inbox := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly})
	jaz.routine(loops.CreateLoop{Name: "Digest", Prompt: "write the digest", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly, Status: loops.StatusPaused})
	jaz.write(bot.ID, "hello")
	slot := time.Now().Add(73 * time.Hour).Truncate(time.Hour)
	jaz.guard(slot.Add(time.Second))

	// A hidden turn in the bot's chat and the bot's own group post are not the
	// user.
	if err := jaz.server.QueueInternalTurn(jaz.ctx, storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("check in quietly")}); err != nil {
		t.Fatal(err)
	}
	jaz.idle(bot.ID)
	if err := jaz.bots.Message(bot.ID, group.ID, "status from the bot"); err != nil {
		t.Fatal(err)
	}
	jaz.guard(time.Now())
	// A message seen only once its own three days have passed, as after Jaz
	// was off that long, starts no welcome back.
	jaz.write(bot.ID, "one more thing")
	jaz.guard(slot.Add(time.Hour))
	if notices := jaz.notices(bot.ID); len(notices) != 1 {
		t.Fatalf("notices = %q, want the routines still waiting", notices)
	}

	if err := jaz.bots.Post(group.ID, "back now", nil); err != nil {
		t.Fatal(err)
	}
	jaz.guard(time.Now())
	notices := jaz.notices(bot.ID)
	if len(notices) != 2 || !strings.Contains(notices[1], "Inbox") || strings.Contains(notices[1], "Digest") {
		t.Fatalf("notices = %q, want the waiting routine back and the paused one left out", notices)
	}
	jaz.dueAt(slot.Add(time.Hour + time.Second))
	jaz.waitRun(jaz.runs(inbox.ID)[0].ID, loops.RunStatusOK)
}

// awayFrom has the user write to a new bot with a routine, then go three days
// without writing, so the bot's routines wait.
func (p *jazProcess) awayFrom(name string) bots.Bot {
	p.t.Helper()
	bot := p.newBot(name)
	p.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly})
	p.write(bot.ID, "hello")
	p.guard(time.Now().Add(73 * time.Hour))
	if notices := p.notices(bot.ID); len(notices) != 1 {
		p.t.Fatalf("notices = %q, want the routines waiting", notices)
	}
	return bot
}

// back reports whether the user's last act let bot's routines run again.
func (p *jazProcess) back(bot string) bool {
	p.t.Helper()
	p.guard(time.Now())
	return len(p.notices(bot)) == 2
}

func TestAnAgentsMessageToATopLevelChatIsNotTheUserComingBack(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.awayFrom("Gimli")
	chat, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "notes", Title: "Notes", Directory: root})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := acp.NewMCPTools(jaz.manager).Send(jaz.ctx, toolCallFrom(bot.ID), acp.MCPSendInput{ThreadID: chat.ID, Prompt: "follow up on the notes"}); err != nil {
		t.Fatal(err)
	}
	jaz.idle(chat.ID)
	if jaz.back(bot.ID) {
		t.Fatal("the bot's message to a chat let its routines run again")
	}
	jaz.write(chat.ID, "thanks")
	if !jaz.back(bot.ID) {
		t.Fatal("the user's own message in the chat did not let the routines run again")
	}
}

func TestAnAnswerToAnAgentsQuestionIsTheUserComingBack(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.awayFrom("Gimli")

	answered := make(chan error, 1)
	go func() {
		_, _, err := acp.NewMCPTools(jaz.manager).AskUser(jaz.ctx, toolCallFrom(bot.ID), acp.AskUserInput{Questions: []acp.UserQuestion{{ID: "when", Question: "When should I check the inbox?", Options: []string{"Now", "Tonight"}}}})
		answered <- err
	}()
	var question string
	jaz.eventually("the question in the bot's chat", func() bool {
		events, err := jaz.store.LoadSessionEvents(bot.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Type == "permission_request" && event.Permission != nil && len(event.Permission.Questions) > 0 {
				question = event.Permission.ID
			}
		}
		return question != ""
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/interactive-response", strings.NewReader(`{"request_id":"`+question+`","answers":{"when":{"answers":["Now"]}}}`))
	res := httptest.NewRecorder()
	jaz.server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("answer status = %d, body = %s", res.Code, res.Body.String())
	}
	if err := <-answered; err != nil {
		t.Fatal(err)
	}
	if !jaz.back(bot.ID) {
		t.Fatal("the user's answer did not let the routines run again")
	}
}

func TestAMessageQueuedBehindABusyTurnIsTheUserComingBack(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.awayFrom("Gimli")
	if err := jaz.server.QueueInternalTurn(jaz.ctx, storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("tidy the notes until released")}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)

	jaz.queueUser(bot.ID, `{"text":"one more thing"}`)
	if !jaz.back(bot.ID) {
		t.Fatal("the message the user queued did not let the routines run again")
	}
	if queued := jaz.queued(bot.ID); len(queued) != 1 || queued[0].Text != "one more thing" {
		t.Fatalf("queue = %+v, want the user's message still waiting", queued)
	}
	jaz.release()
	jaz.idle(bot.ID)
}

// The bots list shows a bot away only while routines of its own wait for the
// user: not one whose routines keep running, nor one with none, and none once
// the user is back.
func TestTheBotsListShowsAwayOnlyBotsWhoseRoutinesWait(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	watching := jaz.newBot("Legolas")
	jaz.routine(loops.CreateLoop{Name: "Monitor", Prompt: "watch the furnace", BotID: watching.ID, Thread: loops.ThreadMain, Schedule: hourly, KeepRunning: true})
	jaz.newBot("Frodo")
	resting := jaz.awayFrom("Gimli")
	away := func() []string {
		t.Helper()
		list, err := jaz.bots.List()
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, bot := range list {
			if bot.Away {
				names = append(names, bot.Name)
			}
		}
		return names
	}
	if names := away(); len(names) != 1 || names[0] != "Gimli" {
		t.Fatalf("away bots = %q, want only Gimli, whose routine waits", names)
	}
	jaz.write(resting.ID, "back now")
	if !jaz.back(resting.ID) {
		t.Fatal("the user's message did not let the routines run again")
	}
	if names := away(); len(names) != 0 {
		t.Fatalf("away bots = %q after the user came back, want none", names)
	}
}

func TestHumanInputBoundariesResumeAwayRoutines(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		body string
		busy bool
	}{
		{"queued", "/queue", `{"op":"append","message":{"text":"back now"}}`, false},
		{"stream", "/messages:stream", `{"message":"back now"}`, false},
		{"stream steer", "/messages:stream", `{"message":"back now"}`, true},
		{"voice input", "/agent/input", `{"message":"back now"}`, false},
		{"voice steer", "/agent/input", `{"message":"back now"}`, true},
		{"interactive text", "/interactive-response", `{"text":"back now"}`, false},
		{"interactive steer", "/interactive-response", `{"text":"back now"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_NATIVE_STEERING": "1"})
			mux := http.NewServeMux()
			mux.HandleFunc("POST /v1/sessions/{session}/agent/input", agentsessions.NewHandler(jaz.manager).Input)
			mux.Handle("/", jaz.server.Handler())
			bot := jaz.awayFrom("Gimli")
			if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "an unmarked background writer"}); err != nil {
				t.Fatal(err)
			}
			jaz.idle(bot.ID)
			if jaz.back(bot.ID) {
				t.Fatal("an unmarked user-role write resumed routines")
			}
			if tc.busy {
				if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "block until cancelled"}); err != nil {
					t.Fatal(err)
				}
				waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+tc.path, strings.NewReader(tc.body))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("input status = %d: %s", response.Code, response.Body.String())
			}
			jaz.idle(bot.ID)
			if !jaz.back(bot.ID) {
				t.Fatal("human input did not resume routines after its queue drained")
			}
		})
	}
}
