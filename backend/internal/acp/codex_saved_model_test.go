package acp_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

// codexPlan is the environment of a Codex account whose plan offers models.
func codexPlan(models string) map[string]string {
	return map[string]string{"JAZ_FAKE_ACP_MODELS": models, "JAZ_FAKE_ACP_SET_CONFIG": "1"}
}

func toolCallFrom(threadID string) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(threadID)}}
}

// turns lists the turns Codex ran since the last call, oldest first.
func (p *jazProcess) turns() []fakeTurn {
	p.t.Helper()
	path := filepath.Join(p.root, "turns.log")
	turns := readFakeTurns(p.t, path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		p.t.Fatal(err)
	}
	return turns
}

// ranOnly reports whether every turn ran on model.
func ranOnly(turns []fakeTurn, model string) bool {
	return len(turns) > 0 && !slices.ContainsFunc(turns, func(turn fakeTurn) bool { return turn.Model != model })
}

func TestSavedCodexModelsStartOnCodexDefaultAfterAPlanWithdrawsThem(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, codexPlan("gpt-6-sol,gpt-6-luna,gpt-6-astra"))
	bot, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "gimli", Title: "Gimli", SourceType: storage.SourceBot, Directory: root, Model: "gpt-6-astra"})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, jaz.ctx, jaz.manager, bot.ID, "hello")
	routine := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Model: "gpt-6-astra"})
	ownerless := jaz.routine(loops.CreateLoop{Name: "Digest", Prompt: "write the digest", Model: "gpt-6-astra"})
	if turns := jaz.turns(); !ranOnly(turns, "gpt-6-astra") {
		t.Fatalf("turns while the plan offered gpt-6-astra = %v", turns)
	}

	jaz = jaz.restart(codexPlan("gpt-6-sol,gpt-6-luna"))
	sendAndWait(t, jaz.ctx, jaz.manager, bot.ID, "hello again")
	_, subtask, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, toolCallFrom(bot.ID), acp.MCPCreateInput{Prompt: "summarise the inbox", Commitment: owe(t, store, bot.ID)})
	if err != nil {
		t.Fatalf("a thread the bot starts on its saved model: %v", err)
	}
	if job, err := jaz.manager.Wait(jaz.ctx, acp.WaitRequest{Session: subtask.ThreadID}); err != nil || job.State != acp.StateIdle || job.Assistant == "" {
		t.Fatalf("subtask = %#v, %v", job, err)
	}
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.waitRun(run.ID, loops.RunStatusOK)
	if _, err := jaz.loops.Update(routine.ID, loops.UpdateLoop{Name: new("Inbox digest")}); err != nil {
		t.Fatalf("renaming a routine whose saved model was withdrawn: %v", err)
	}
	if err := jaz.bots.AdoptLoops(jaz.ctx); err != nil {
		t.Fatalf("adopting a routine whose saved model was withdrawn: %v", err)
	}
	adopted, err := jaz.loops.Load(ownerless.ID)
	if err != nil {
		t.Fatal(err)
	}
	if owner, err := store.LoadSession(adopted.BotID); err != nil || owner.Model != "gpt-6-astra" {
		t.Fatalf("adopting bot = %#v, %v; want it on the routine's saved model", owner, err)
	}
	// Only the bot's first turn started before Jaz saw the plan's models.
	turns := jaz.turns()
	if len(turns) < 3 || !ranOnly(turns, "gpt-6-sol") || slices.ContainsFunc(turns[1:], func(turn fakeTurn) bool { return turn.Switched }) {
		t.Fatalf("turns after the plan withdrew gpt-6-astra = %v; want all on Codex's default gpt-6-sol, started there", turns)
	}
}

func TestRoutineToolsRefuseAModelCodexDoesNotOffer(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, codexPlan("gpt-6-sol,gpt-6-luna"))
	bot := jaz.bot("Gimli")
	sendAndWait(t, jaz.ctx, jaz.manager, bot.ID, "hello")
	tools := loops.NewMCPTools(jaz.loops)
	create := loops.MCPCreateInput{
		Prompt:   "check the inbox",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		ACPAgent: acp.AgentCodex,
		Model:    "gpt-6-astra",
		Bot:      bot.ID,
	}

	if _, _, err := tools.Create(jaz.ctx, toolCallFrom(bot.ID), create); err == nil || !strings.Contains(err.Error(), "available model ids: gpt-6-luna, gpt-6-sol") {
		t.Fatalf("loop_create with a model Codex does not offer: %v", err)
	}
	if saved, err := jaz.loops.List(); err != nil || len(saved) != 0 {
		t.Fatalf("refused loop_create saved %#v, %v", saved, err)
	}
	create.Model = "gpt-6-sol"
	_, routine, err := tools.Create(jaz.ctx, toolCallFrom(bot.ID), create)
	if err != nil || routine.Model != "gpt-6-sol" {
		t.Fatalf("loop_create with a model Codex offers = %#v, %v", routine, err)
	}
	if _, _, err := tools.Update(jaz.ctx, toolCallFrom(bot.ID), loops.MCPUpdateInput{ID: routine.ID, Model: new("gpt-6-astra")}); err == nil || !strings.Contains(err.Error(), "available model ids: gpt-6-luna, gpt-6-sol") {
		t.Fatalf("loop_update to a model Codex does not offer: %v", err)
	}
	if saved, err := jaz.loops.Load(routine.ID); err != nil || saved.Model != "gpt-6-sol" {
		t.Fatalf("refused loop_update changed the routine to %#v, %v", saved, err)
	}
}
