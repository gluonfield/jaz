package jaztools

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/threads"
	"github.com/wins/jaz/backend/internal/widgets"
)

type fakeACPService struct {
	spawned chan acp.SpawnRequest
}

func (s fakeACPService) AskUser(context.Context, string, acp.AskUserInput) (acp.AskUserOutput, error) {
	return acp.AskUserOutput{}, nil
}

func (s fakeACPService) Start(_ context.Context, req acp.SpawnRequest, _ storage.QueuedMessage) (acp.Job, error) {
	s.spawned <- req
	return acp.Job{ID: "child", Slug: req.Slug, ACPAgent: req.ACPAgent, State: acp.StateStarting}, nil
}

func (s fakeACPService) Input(context.Context, acp.SteerRequest) (acp.Job, error) {
	return acp.Job{}, nil
}

func (s fakeACPService) WaitThreads(context.Context, []string, time.Duration) (acp.ThreadResults, error) {
	return acp.ThreadResults{}, nil
}

func (s fakeACPService) Cancel(context.Context, string) (acp.Job, error) {
	return acp.Job{}, nil
}

func (s fakeACPService) Agents() []string {
	return []string{acp.AgentCodex, acp.AgentJaz}
}

func (s fakeACPService) AgentOptions(req acp.AgentOptionsRequest) (acp.AgentOptionsOutput, error) {
	agents := acp.SelectableAgentNames(s.Agents())
	if req.Agent != "" && acp.CanonicalAgentName(req.Agent) == acp.AgentCodex {
		agents = []string{acp.AgentCodex}
	}
	out := acp.AgentOptionsOutput{Agents: make([]acp.AgentSpawnOptions, 0, len(agents))}
	for _, agent := range agents {
		out.Agents = append(out.Agents, acp.AgentSpawnOptions{Name: agent})
	}
	return out, nil
}

func TestThreadToolsCreateDiscoverAndRead(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "memory.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })

	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jaztools"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
		testCalendarTools(t, store),
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))
	agentService := fakeACPService{spawned: make(chan acp.SpawnRequest, 1)}
	service.SetAgents(agentService)
	service.SetThreads(threads.NewService(sqlitestore.NewSearchQueries(store), store))

	target, err := store.CreateSession(storage.CreateSession{
		Slug: "review-target", Title: "Review target", Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: acp.AgentCodex},
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-time.Minute)
	if err := store.AppendMessageRecords(target.ID,
		storage.Message{Role: "user", Content: "Please review the checkout bug.", CreatedAt: start},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(target.ID, sessionevents.Event{
		Type: sessionevents.TypeACPMessage, Content: "Patched checkout and verified tests.", At: start.Add(time.Second),
		ACP: &sessionevents.ACPEvent{ID: target.ID, TextRunID: "message:answer"},
	}); err != nil {
		t.Fatal(err)
	}

	session, closeSession := connectClient(t, service.Server())
	defer closeSession()
	for _, discovery := range []struct {
		name string
		args map[string]any
	}{
		{"list_threads", map[string]any{"limit": 20}},
		{"search_threads", map[string]any{"query": "patched"}},
	} {
		call, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: discovery.name, Arguments: discovery.args})
		if err != nil {
			t.Fatal(err)
		}
		result := structured[threads.DiscoveryResponse](t, call)
		found := false
		for _, thread := range result.Threads {
			if thread.ThreadID == target.ID {
				found = true
				if thread.Title != "Review target" {
					t.Fatalf("lost thread title: %#v", thread)
				}
				if discovery.name == "search_threads" && thread.Snippet == "" {
					t.Fatal("search did not return message evidence")
				}
			}
		}
		if !found {
			t.Fatalf("%s omitted persisted thread with no live agent: %#v", discovery.name, result)
		}
	}

	threadCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_thread",
		Arguments: map[string]any{"threadId": target.ID, "limit": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	threadContext := structured[threads.ContextResponse](t, threadCall)
	if threadContext.Session.ID != target.ID || len(threadContext.Messages) != 1 || threadContext.Messages[0].Text != "Patched checkout and verified tests." {
		t.Fatalf("thread context = %#v", threadContext)
	}

	tool := findTool(t, session, "create_thread")
	if tool == nil {
		t.Fatal("create_thread not advertised")
	}
	schema, _ := tool.InputSchema.(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	for _, name := range []string{"agent", "modelProvider", "model", "thinking", "prompt"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("create_thread schema missing %s: %#v", name, properties)
		}
	}

	call, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_thread",
		Arguments: map[string]any{
			"agent":         acp.AgentCodex,
			"prompt":        "Review changes",
			"slug":          "child",
			"modelProvider": "openai",
			"model":         "gpt-5.5",
			"thinking":      "high",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.IsError {
		t.Fatalf("create_thread returned error: %#v", call)
	}
	select {
	case req := <-agentService.spawned:
		if req.ACPAgent != acp.AgentCodex || req.ModelProvider != "openai" || req.Model != "gpt-5.5" || req.ReasoningEffort != "high" {
			t.Fatalf("spawn request = %#v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("create_thread did not reach ACP service")
	}
}
