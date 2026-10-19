package acp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// modelSets counts the times Codex was asked to switch a session to model.
func (p *jazProcess) modelSets(model string) int {
	p.t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.root, "requests.jsonl"))
	if err != nil && !os.IsNotExist(err) {
		p.t.Fatal(err)
	}
	count := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, `"method":"session/set_config_option"`) && strings.Contains(line, `"value":"`+model+`"`) {
			count++
		}
	}
	return count
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
	if jaz.modelSets("gpt-6-astra") != 1 {
		t.Fatal("the bot did not run gpt-6-astra while the plan offered it")
	}

	jaz = jaz.restart(codexPlan("gpt-6-sol,gpt-6-luna"))
	sendAndWait(t, jaz.ctx, jaz.manager, bot.ID, "hello again")
	_, subtask, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, toolCallFrom(bot.ID), acp.MCPCreateInput{Prompt: "summarise the inbox", Commitment: owe(t, store, bot.ID)})
	if err != nil {
		t.Fatalf("a thread the bot starts on its saved model: %v", err)
	}
	if done, err := jaz.manager.WaitThreads(jaz.ctx, []string{subtask.ThreadID}, 10*time.Second); err != nil || len(done.Threads) != 1 || done.Threads[0].State != acp.StateIdle || done.Threads[0].Assistant == "" {
		t.Fatalf("subtask = %#v, %v", done, err)
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
	if jaz.modelSets("gpt-6-astra") != 1 {
		t.Fatal("a session asked Codex for gpt-6-astra after the plan withdrew it")
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
