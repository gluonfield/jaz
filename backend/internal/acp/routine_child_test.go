package acp_test

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

// reports lists the routine reports a bot was handed, in order.
func (p *jazProcess) reports() []string {
	var reports []string
	for _, prompt := range p.prompts() {
		if strings.Contains(prompt, "pass it on to the user") {
			reports = append(reports, prompt)
		}
	}
	return reports
}

// say sends text from a run's thread the way its agent's send_message does.
func (p *jazProcess) say(runID, text string) {
	p.t.Helper()
	child := p.waitRun(runID, loops.RunStatusRunning).ThreadID
	waitForBlockingTool(p.t, p.ctx, p.manager, child)
	if err := p.bots.Say(child, text); err != nil {
		p.t.Fatal(err)
	}
}

func TestABotsRoutinesRunAtOnceInChildThreadsOfItsHome(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1", "JAZ_FAKE_ACP_SET_MODEL": "1", "JAZ_FAKE_ACP_SET_CONFIG": "1"})
	bot, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "gimli", Title: "Gimli", SourceType: storage.SourceBot, Directory: t.TempDir(), Model: "bot-model", ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	var runs []loops.Run
	for _, name := range []string{"Inbox", "Notes"} {
		routine, err := jaz.loops.Create(loops.CreateLoop{Name: name, Prompt: "sync until released", BotID: bot.ID, Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"}})
		if err != nil {
			t.Fatal(err)
		}
		run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run)
	}
	for i, run := range runs {
		jaz.say(run.ID, []string{"Two new emails.", "One new note."}[i])
	}
	for _, run := range runs {
		session, err := store.LoadSession(jaz.run(run.ID).ThreadID)
		if err != nil || session.ParentID != bot.ID || session.RuntimeRef.Cwd != bot.RuntimeRef.Cwd || session.RuntimeRef.Agent != bot.RuntimeRef.Agent || session.Model != "bot-model" || session.ReasoningEffort != "high" {
			t.Fatalf("child = %+v %+v, %v; want a thread of the bot in its home on its agent, model and effort", session, session.RuntimeRef, err)
		}
	}
	if session, err := store.LoadSession(bot.ID); err != nil || session.Status == storage.StatusRunning {
		t.Fatalf("bot = %+v, %v; want its own thread free while both routines run", session, err)
	}

	jaz.release()
	for _, run := range runs {
		jaz.waitRun(run.ID, loops.RunStatusOK)
	}
	jaz.eventually("both reports reaching the bot", func() bool { return len(jaz.reports()) == 2 })
	if reports := strings.Join(jaz.reports(), "\n"); !strings.Contains(reports, "[routine] Inbox") || !strings.Contains(reports, "Two new emails.") || !strings.Contains(reports, "One new note.") {
		t.Fatalf("reports = %q", reports)
	}
}

func TestAQuietChildRunLeavesItsBotAlone(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	quiet := jaz.routine(loops.CreateLoop{Name: "Notes", Prompt: "check the notes", BotID: bot.ID})
	run, err := jaz.loops.RunNow(jaz.ctx, quiet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if jaz.waitRun(run.ID, loops.RunStatusOK).Delivered {
		t.Fatal("a quiet run delivered")
	}
	talking := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox until released", BotID: bot.ID})
	run, err = jaz.loops.RunNow(jaz.ctx, talking.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.say(run.ID, "Two new emails.")
	jaz.release()
	if !jaz.waitRun(run.ID, loops.RunStatusOK).Delivered {
		t.Fatal("a run that reported to its bot did not deliver")
	}
	jaz.eventually("the talking run's report", func() bool { return len(jaz.reports()) == 1 })
	if prompts := jaz.prompts(); len(prompts) != 3 || !strings.Contains(prompts[2], "[routine] Inbox") {
		t.Fatalf("prompts = %q, want two runs and one report to the bot", prompts)
	}
}

func TestRestartMidChildRunResumesItAndReportsToTheBotOnce(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Report", Prompt: "write the report until released", BotID: bot.ID})
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.say(run.ID, "The report is ready.")
	child := jaz.run(run.ID).ThreadID

	jaz = jaz.restart(nil)
	jaz.waitRun(run.ID, loops.RunStatusOK)
	jaz.eventually("the resumed run's report", func() bool { return len(jaz.reports()) == 1 })
	if session, err := store.LoadSession(child); err != nil || session.ParentID != bot.ID {
		t.Fatalf("child = %+v, %v", session, err)
	}
	if reports := jaz.reports(); !strings.Contains(reports[0], "The report is ready.") {
		t.Fatalf("report = %q", reports[0])
	}
}

func TestARoutineMovesToAChildWhenItsBotsThreadIsFull(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	for _, fill := range []storage.Usage{{ContextTokens: 90_000, ContextWindowTokens: 100_000}, {ContextTokens: 90_000}} {
		bot := jaz.bot("Gimli")
		bot.Usage = fill
		if err := store.SaveSession(bot); err != nil {
			t.Fatal(err)
		}
		routine := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain})
		run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
		if err != nil {
			t.Fatal(err)
		}
		ran := jaz.waitRun(run.ID, loops.RunStatusOK)
		moved := ran.ThreadID != bot.ID
		if want := fill.ContextWindowTokens > 0; moved != want {
			t.Fatalf("with usage %+v the run went to %s, moved = %v, want %v", fill, ran.ThreadID, moved, want)
		}
	}
}
