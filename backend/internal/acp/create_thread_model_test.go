package acp

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// advertisingAgent is an agent whose sessions advertise fewer models than
// Jaz's catalog suggests for it.
type advertisingAgent struct {
	name       string
	advertised []string
	missing    string
}

// rejectingAgents refuse to start a session on a model they do not advertise;
// Codex starts on its own default instead.
var (
	rejectingAgents = []advertisingAgent{
		{name: AgentGrok, advertised: []string{"grok-4.5", "grok-4.6"}, missing: "grok-4.7"},
		{name: AgentClaude, advertised: []string{"opus[1m]", "sonnet"}, missing: "haiku"},
	}
	advertisingAgents = append(rejectingAgents, advertisingAgent{name: AgentCodex, advertised: []string{"gpt-6-sol", "gpt-6-luna"}, missing: "gpt-6-astra"})
)

func threadToolsFor(t *testing.T, agent advertisingAgent) (*mcp.ClientSession, *sqlitestore.Store, string) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	manager := NewManager(store, Config{
		Root:         t.TempDir(),
		Workspace:    t.TempDir(),
		ModelCatalog: modelcatalog.NewService(nil),
		Agents: map[string]AgentConfig{
			agent.name: {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":       "1",
					"JAZ_FAKE_ACP_SET_CONFIG":  "1",
					"JAZ_FAKE_ACP_MODELS":      strings.Join(agent.advertised, ","),
					"JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
				},
			},
		},
	}, log.New(io.Discard))
	manager.Events = sessionevents.New()
	t.Cleanup(manager.Close)
	server := mcp.NewServer(&mcp.Implementation{Name: "jaz", Version: "test"}, nil)
	NewMCPTools(manager).AddTo(server)
	client, closeClient := connectMCPClient(t, server)
	t.Cleanup(closeClient)
	return client, store, requestLog
}

func callThreadTool(t *testing.T, client *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func listedModels(t *testing.T, client *mcp.ClientSession, agent string) modelValues {
	t.Helper()
	out := structuredContent[AgentOptionsOutput](t, callThreadTool(t, client, ToolListAgentOptions, map[string]any{"agent": agent}))
	if len(out.Agents) != 1 {
		t.Fatalf("agents = %#v", out.Agents)
	}
	return out.Agents[0].Models
}

func storedSessions(t *testing.T, store *sqlitestore.Store, archived bool) []storage.Session {
	t.Helper()
	sessions, err := store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true, Archived: archived})
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

func agentRequests(t *testing.T, requestLog, method string) int {
	t.Helper()
	raw, err := os.ReadFile(requestLog)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.Count(string(raw), `"method":"`+method+`"`)
}

func TestCreateThreadWithModelAgentDoesNotAdvertiseLeavesNoThread(t *testing.T) {
	for _, agent := range rejectingAgents {
		t.Run(agent.name, func(t *testing.T) {
			client, store, requestLog := threadToolsFor(t, agent)
			create := map[string]any{"prompt": "Review the bot design", "agent": agent.name, "model": agent.missing, "slug": "bots-design-council"}

			res := callThreadTool(t, client, ToolCreateThread, create)
			if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "not advertised") {
				t.Fatalf("first create_thread = %#v", res.Content)
			}
			if visible := storedSessions(t, store, false); len(visible) != 0 {
				t.Fatalf("failed create_thread left threads in the sidebar: %#v", visible)
			}
			failed := storedSessions(t, store, true)
			if len(failed) != 1 || failed[0].Status != storage.StatusError || !strings.Contains(failed[0].Error, "not advertised") {
				t.Fatalf("failed thread = %#v", failed)
			}

			models := listedModels(t, client, agent.name)
			if models.contains(agent.missing) || len(models) != len(agent.advertised) {
				t.Fatalf("list_agent_options offers models %s did not advertise: %#v", agent.name, models)
			}

			starts := agentRequests(t, requestLog, "initialize")
			res = callThreadTool(t, client, ToolCreateThread, create)
			if !res.IsError || res.Content[0].(*mcp.TextContent).Text != failed[0].Error {
				t.Fatalf("retry was refused for another reason than the agent's session: %#v", res.Content)
			}
			if all := append(storedSessions(t, store, false), storedSessions(t, store, true)...); len(all) != 1 {
				t.Fatalf("rejected retry created a thread: %#v", all)
			}
			if agentRequests(t, requestLog, "initialize") != starts {
				t.Fatalf("rejected retry started %s", agent.name)
			}
		})
	}
}

func TestCreateThreadStartsModelAgentAdvertises(t *testing.T) {
	for _, agent := range advertisingAgents {
		t.Run(agent.name, func(t *testing.T) {
			client, store, requestLog := threadToolsFor(t, agent)
			for i, model := range agent.advertised {
				created := structuredContent[ThreadSnapshot](t, callThreadTool(t, client, ToolCreateThread, map[string]any{
					"prompt": "Review the bot design", "agent": agent.name, "model": model,
				}))
				if created.Agent != agent.name || created.Model != model {
					t.Fatalf("created = %#v", created)
				}
				if i == 0 && listedModels(t, client, agent.name).contains(agent.missing) {
					t.Fatalf("list_agent_options still offers %s after %s advertised its models", agent.missing, agent.name)
				}
				done := structuredContent[MCPWaitOutput](t, callThreadTool(t, client, ToolWaitThreads, map[string]any{"targets": []map[string]any{{"threadId": created.ThreadID}}}))
				if len(done.Threads) != 1 || done.Threads[0].State != StateIdle || done.Threads[0].Assistant == "" {
					t.Fatalf("thread did not run its prompt: %#v", done)
				}
			}
			if visible := storedSessions(t, store, false); len(visible) != len(agent.advertised) {
				t.Fatalf("visible threads = %#v", visible)
			}
			if agentRequests(t, requestLog, "session/prompt") != len(agent.advertised) {
				t.Fatalf("prompts did not reach %s", agent.name)
			}
		})
	}
}
