package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type fakeMCPService struct {
	spawned SpawnRequest
	job     Job
	list    []Job
	agents  []string
}

func (s *fakeMCPService) Spawn(_ context.Context, req SpawnRequest) (SpawnResult, error) {
	s.spawned = req
	return SpawnResult{Status: "ok", SessionID: "child", Slug: req.Slug, ACPAgent: req.ACPAgent, State: StateIdle}, nil
}

func (s *fakeMCPService) Send(context.Context, SendRequest) (Job, error) {
	return s.job, nil
}

func (s *fakeMCPService) Status(string) (Job, error) {
	return s.job, nil
}

func (s *fakeMCPService) Wait(context.Context, WaitRequest) (Job, error) {
	return s.job, nil
}

func (s *fakeMCPService) Cancel(context.Context, string) (Job, error) {
	return s.job, nil
}

func (s *fakeMCPService) List() []Job {
	return append([]Job(nil), s.list...)
}

func (s *fakeMCPService) Agents() []string {
	if s.agents != nil {
		return append([]string(nil), s.agents...)
	}
	return []string{AgentCodex, AgentJaz}
}

func (s *fakeMCPService) AgentOptions(req AgentOptionsRequest) (AgentOptionsOutput, error) {
	agents := SelectableAgentNames(s.Agents())
	if req.Agent != "" {
		agents = filterAgentNames(agents, CanonicalAgentName(req.Agent))
	}
	out := AgentOptionsOutput{Agents: make([]AgentSpawnOptions, 0, len(agents))}
	for _, agent := range agents {
		out.Agents = append(out.Agents, AgentSpawnOptions{
			Name:   agent,
			Models: []AgentModelOption{{Model: "gpt-5.5", Label: "GPT-5.5"}},
		})
	}
	return out, nil
}

func TestMCPSpawnAcceptsAgentNameAliasAndModelOverrides(t *testing.T) {
	service := &fakeMCPService{}
	tools := NewMCPTools(service)
	header := http.Header{}
	header.Set(mcpsession.HeaderName, "parent-session")
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: header}}

	_, result, err := tools.Spawn(context.Background(), req, MCPSpawnInput{
		AgentName:       AgentCodex,
		Slug:            "review",
		ModelProvider:   "openai",
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ACPAgent != AgentCodex || service.spawned.ACPAgent != AgentCodex {
		t.Fatalf("agent alias was not forwarded: result=%#v request=%#v", result, service.spawned)
	}
	if service.spawned.ModelProvider != "openai" || service.spawned.Model != "gpt-5.5" || service.spawned.ReasoningEffort != "high" {
		t.Fatalf("model overrides were not forwarded: %#v", service.spawned)
	}
	if service.spawned.ParentID != "parent-session" {
		t.Fatalf("parent session was not forwarded: %#v", service.spawned)
	}
}

func TestSpawnInputSchemaAdvertisesAgentEnums(t *testing.T) {
	schema := spawnInputSchema([]string{AgentCodex, AgentClaude})
	properties, _ := schema["properties"].(map[string]any)
	for _, name := range []string{"acp_agent", "agent_name"} {
		property, _ := properties[name].(map[string]any)
		enum, _ := property["enum"].([]string)
		if len(enum) != 2 || enum[0] != AgentCodex || enum[1] != AgentClaude {
			t.Fatalf("%s enum = %#v", name, property["enum"])
		}
	}
	effort, _ := properties["reasoning_effort"].(map[string]any)
	effortEnum, _ := effort["enum"].([]string)
	if len(effortEnum) == 0 ||
		!slices.Contains(effortEnum, "xhigh") ||
		!slices.Contains(effortEnum, "ultra") ||
		!slices.Contains(effortEnum, "ultracode") {
		t.Fatalf("reasoning_effort enum = %#v", effort["enum"])
	}
}

func TestMCPAvailableAgentsFiltersJaz(t *testing.T) {
	agents := NewMCPTools(&fakeMCPService{}).availableAgents()
	if len(agents) != 1 || agents[0] != AgentCodex {
		t.Fatalf("agents = %#v", agents)
	}
}

func TestMCPAvailableAgentsDoesNotInventFallbacks(t *testing.T) {
	agents := NewMCPTools(&fakeMCPService{agents: []string{}}).availableAgents()
	if len(agents) != 0 {
		t.Fatalf("agents = %#v, want none", agents)
	}
}

func TestMCPAdvertisesAgentOptionsSeparatelyFromAgentList(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	NewMCPTools(&fakeMCPService{}).AddTo(server)
	session, closeSession := connectMCPClient(t, server)
	defer closeSession()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{ToolACPAgentOptions, ToolACPSessionList} {
		if !names[name] {
			t.Fatalf("missing tool %s in %#v", name, names)
		}
	}
}

func TestResolveAgentSelectorRejectsConflictingAliases(t *testing.T) {
	if _, err := ResolveAgentSelector(AgentCodex, AgentClaude); err == nil {
		t.Fatal("expected conflicting agent aliases to fail")
	}
}

func TestMCPAgentJobOutputValidatesToolCallRawInputObject(t *testing.T) {
	job := Job{
		ID:              "child",
		Slug:            "physicslab-plan-claude-review",
		ACPAgent:        AgentClaude,
		ACPSession:      "claude-session",
		ModelProvider:   AgentClaude,
		Model:           "claude-opus-4-8",
		ReasoningEffort: "xhigh",
		State:           StateIdle,
		ToolCalls: []sessionevents.ACPToolCall{{
			ID:       "tool-1",
			Title:    "Read plan.html",
			Status:   "completed",
			Kind:     "read",
			ToolName: "Read",
			RawInput: map[string]any{
				"file_path": "/tmp/plan.html",
				"nested":    map[string]any{"limit": 1},
			},
		}},
	}
	service := &fakeMCPService{job: job, list: []Job{job}}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	NewMCPTools(service).AddTo(server)
	session, closeSession := connectMCPClient(t, server)
	defer closeSession()

	for _, name := range []string{ToolACPSessionStatus, ToolACPSessionWait, ToolACPSessionCancel} {
		call, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      name,
			Arguments: map[string]any{"session": "child"},
		})
		if err != nil {
			t.Fatal(err)
		}
		output := structuredContent[Job](t, call)
		if got := output.ToolCalls[0].RawInput["file_path"]; got != "/tmp/plan.html" {
			t.Fatalf("%s raw_input file_path = %#v", name, got)
		}
		if nested, ok := output.ToolCalls[0].RawInput["nested"].(map[string]any); !ok || nested["limit"] != float64(1) {
			t.Fatalf("%s raw_input nested = %#v", name, output.ToolCalls[0].RawInput["nested"])
		}
		if output.ModelProvider != AgentClaude || output.Model != "claude-opus-4-8" || output.ReasoningEffort != "xhigh" {
			t.Fatalf("%s model metadata = %#v", name, output)
		}
	}

	listCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: ToolACPSessionList})
	if err != nil {
		t.Fatal(err)
	}
	list := structuredContent[MCPListOutput](t, listCall)
	if got := list.Sessions[0].ToolCalls[0].RawInput["file_path"]; got != "/tmp/plan.html" {
		t.Fatalf("list raw_input file_path = %#v", got)
	}
	if list.Sessions[0].ModelProvider != AgentClaude || list.Sessions[0].ReasoningEffort != "xhigh" {
		t.Fatalf("list model metadata = %#v", list.Sessions[0])
	}

	optionsCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      ToolACPAgentOptions,
		Arguments: map[string]any{"agent": AgentCodex, "name": "gpt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	options := structuredContent[AgentOptionsOutput](t, optionsCall)
	if len(options.Agents) != 1 || options.Agents[0].Name != AgentCodex {
		t.Fatalf("option agents = %#v", options.Agents)
	}
	if options.Agents[0].Models[0].Model != "gpt-5.5" {
		t.Fatalf("option models = %#v", options.Agents[0].Models)
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
