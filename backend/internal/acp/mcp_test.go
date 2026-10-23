package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

type fakeMCPService struct {
	spawned SpawnRequest
	first   storage.QueuedMessage
	sent    SendRequest
	job     Job
	agents  []string
	refs    []string
	timeout time.Duration
}

func (s *fakeMCPService) AskUser(context.Context, string, AskUserInput) (AskUserOutput, error) {
	return AskUserOutput{}, nil
}

func (s *fakeMCPService) Start(_ context.Context, req SpawnRequest, first storage.QueuedMessage) (Job, error) {
	s.spawned = req
	s.first = first
	return s.job, nil
}

func (s *fakeMCPService) Input(_ context.Context, req SteerRequest) (Job, error) {
	s.sent = req
	return s.job, nil
}

func (s *fakeMCPService) WaitThreads(_ context.Context, refs []string, timeout time.Duration) (ThreadResults, error) {
	s.refs = refs
	s.timeout = timeout
	return ThreadResults{Threads: []Job{s.job}}, nil
}

func (s *fakeMCPService) Cancel(context.Context, string) (Job, error) {
	return s.job, nil
}

func (s *fakeMCPService) AgentOptions(AgentOptionsRequest) (AgentOptionsOutput, error) {
	names := s.agents
	if names == nil {
		names = []string{AgentCodex}
	}
	out := AgentOptionsOutput{}
	for _, name := range names {
		out.Agents = append(out.Agents, AgentSpawnOptions{Name: name})
	}
	return out, nil
}

func TestCreateThreadStartsPromptAndPreservesParentAndSettings(t *testing.T) {
	service := &fakeMCPService{job: Job{ID: "child", ACPAgent: AgentCodex, State: StateStarting}}
	tool := NewMCPTools(service)
	header := http.Header{}
	header.Set(mcpsession.HeaderName, "parent")
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: header}}
	_, _, err := tool.Create(context.Background(), req, MCPCreateInput{
		Prompt: "Review the changes", Agent: AgentCodex, Model: "requested-model",
		ModelProvider: "openai", Thinking: "high", Directory: "project", Plan: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.spawned.ParentID != "parent" || service.spawned.ACPAgent != AgentCodex ||
		service.spawned.Model != "requested-model" || service.spawned.ModelProvider != "openai" ||
		service.spawned.ReasoningEffort != "high" || service.spawned.Directory != "project" {
		t.Fatalf("spawn = %#v", service.spawned)
	}
	if service.first.Text != "Review the changes" || service.first.From != "parent" || !service.first.PlanRequested {
		t.Fatalf("first prompt = %#v", service.first)
	}
}

func TestCreateThreadRejectsEmptyPromptBeforeCreating(t *testing.T) {
	service := &fakeMCPService{}
	_, _, err := NewMCPTools(service).Create(context.Background(), nil, MCPCreateInput{Prompt: "  ", Title: "should not exist"})
	if err == nil || service.spawned.Title != "" {
		t.Fatalf("err = %v, spawn = %#v", err, service.spawned)
	}
}

func TestCreateThreadUsesTheCurrentAgentCatalogueAfterMCPRegistration(t *testing.T) {
	service := &fakeMCPService{agents: []string{AgentCodex}, job: Job{ID: "child", ACPAgent: AgentClaude, State: StateStarting}}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	NewMCPTools(service).AddTo(server)
	client, closeClient := connectMCPClient(t, server)
	defer closeClient()
	service.agents = []string{AgentCodex, AgentClaude}
	call, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: ToolListAgentOptions, Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	options := structuredContent[AgentOptionsOutput](t, call)
	if len(options.Agents) != 2 || options.Agents[1].Name != AgentClaude {
		t.Fatalf("new agent was not discoverable: %+v", options)
	}
	call, err = client.CallTool(t.Context(), &mcp.CallToolParams{Name: ToolCreateThread, Arguments: map[string]any{
		"prompt": "Review the changes", "agent": AgentClaude, "thinking": "provider-owned-effort",
	}})
	if err != nil || call.IsError {
		t.Fatalf("newly advertised agent was rejected by the old schema: %+v, %v", call, err)
	}
	if service.spawned.ACPAgent != AgentClaude || service.spawned.ReasoningEffort != "provider-owned-effort" {
		t.Fatalf("provider settings were changed: %+v", service.spawned)
	}
}

func TestMCPThreadToolsUseCodexNamesAndCompactSnapshots(t *testing.T) {
	service := &fakeMCPService{job: Job{
		ID: "child", State: StateIdle, ACPAgent: AgentClaude,
		Model: "provider-model", ReasoningEffort: "high",
		Assistant: strings.Repeat("x", 5000) + "finished", Thought: "private reasoning",
	}}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	NewMCPTools(service).AddTo(server)
	client, closeClient := connectMCPClient(t, server)
	defer closeClient()
	list, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{ToolCreateThread, ToolSendMessageToThread, ToolWaitThreads, ToolStopThread, ToolListAgentOptions} {
		if !names[name] {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"jazagent_spawn", "jazagent_send", "jazagent_status", "jazagent_wait", "jazagent_cancel", "jazagent_options", "jazagent_list"} {
		if names[name] {
			t.Fatalf("legacy tool %s advertised", name)
		}
	}
	for _, timeout := range []*int{nil, new(0), new(250)} {
		args := map[string]any{"targets": []map[string]string{{"threadId": "child"}}}
		// The default wait ends inside MCP clients' 60 s tool-call budget.
		want := 50 * time.Second
		if timeout != nil {
			args["timeoutMs"] = *timeout
			want = time.Duration(*timeout) * time.Millisecond
		}
		call, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: ToolWaitThreads, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		out := structuredContent[MCPWaitOutput](t, call)
		if service.timeout != want || len(service.refs) != 1 || service.refs[0] != "child" {
			t.Fatalf("wait arguments = %v %v", service.refs, service.timeout)
		}
		thread := out.Threads[0]
		if thread.ThreadID != "child" || thread.Model != "provider-model" || thread.Thinking != "high" ||
			!thread.Truncated || len(thread.Assistant) != 4000 || !strings.HasSuffix(thread.Assistant, "finished") {
			t.Fatalf("snapshot = %#v", thread)
		}
	}
	for _, timeout := range []int{-1, maxWaitMs + 1} {
		call, err := client.CallTool(context.Background(), &mcp.CallToolParams{
			Name: ToolWaitThreads, Arguments: map[string]any{"targets": []map[string]string{{"threadId": "child"}}, "timeoutMs": timeout},
		})
		if err == nil && !call.IsError {
			t.Fatalf("accepted invalid timeout %d", timeout)
		}
	}
	call, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: ToolSendMessageToThread, Arguments: map[string]any{"threadId": "child", "prompt": "continue"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = structuredContent[ThreadSnapshot](t, call)
	if service.sent.Session != "child" || service.sent.Message != "continue" {
		t.Fatalf("follow-up = %#v", service.sent)
	}
}

func connectMCPClient(t *testing.T, server *mcp.Server) (*mcp.ClientSession, func()) {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatal(err)
	}
	return clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func structuredContent[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %#v", res.Content)
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
