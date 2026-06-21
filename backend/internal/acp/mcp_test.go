package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

type fakeMCPService struct {
	spawned SpawnRequest
	job     Job
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
	if s.job.ID == "" {
		return nil
	}
	return []Job{s.job}
}

func (s *fakeMCPService) Agents() []string {
	return []string{AgentCodex, AgentJaz}
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

func TestMCPAgentStatusAndListAcceptObjectRawInput(t *testing.T) {
	service := &fakeMCPService{job: Job{
		ID:         "child",
		Slug:       "physics-review",
		ACPAgent:   AgentClaude,
		ACPSession: "claude-session",
		State:      StateCancelled,
		ToolCalls: []ToolCallSnapshot{{
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
	}}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	NewMCPTools(service).AddTo(server)
	session, closeSession := connectMCPClient(t, server)
	defer closeSession()

	statusCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      MCPToolAgentStatus,
		Arguments: map[string]any{"session": "child"},
	})
	if err != nil {
		t.Fatal(err)
	}
	status := structuredContent[Job](t, statusCall)
	if status.ToolCalls[0].RawInput["file_path"] != "/tmp/plan.html" {
		t.Fatalf("raw_input was not decoded as an object: %#v", status.ToolCalls[0].RawInput)
	}

	listCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: MCPToolAgentList})
	if err != nil {
		t.Fatal(err)
	}
	list := structuredContent[MCPListOutput](t, listCall)
	if len(list.Sessions) != 1 || len(list.Sessions[0].ToolCalls) != 1 {
		t.Fatalf("list summary = %#v", list)
	}
}

func TestSpawnInputSchemaAdvertisesAgentEnums(t *testing.T) {
	schema := spawnInputSchema([]string{AgentCodex, AgentJaz})
	properties, _ := schema["properties"].(map[string]any)
	for _, name := range []string{"acp_agent", "agent_name"} {
		property, _ := properties[name].(map[string]any)
		enum, _ := property["enum"].([]string)
		if len(enum) != 2 || enum[0] != AgentCodex || enum[1] != AgentJaz {
			t.Fatalf("%s enum = %#v", name, property["enum"])
		}
	}
}

func TestResolveAgentSelectorRejectsConflictingAliases(t *testing.T) {
	if _, err := ResolveAgentSelector(AgentCodex, AgentClaude); err == nil {
		t.Fatal("expected conflicting agent aliases to fail")
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
