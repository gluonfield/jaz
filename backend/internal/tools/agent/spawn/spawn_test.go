package spawn

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/tools"
)

type createService struct {
	acp.MCPService
	spawned acp.SpawnRequest
	first   storage.QueuedMessage
}

func (s *createService) Agents() []string {
	return []string{acp.AgentCodex}
}

func (s *createService) Start(_ context.Context, req acp.SpawnRequest, first storage.QueuedMessage) (acp.Job, error) {
	s.spawned = req
	s.first = first
	return acp.Job{ID: "child", State: acp.StateStarting}, nil
}

func TestCreateThreadSharesMCPSchemaAndPreservesNativeCaller(t *testing.T) {
	service := &createService{}
	tool := &Tool{Service: service}
	def := tool.Definition()
	mcp := acp.NewMCPTools(service).CreateDefinition()
	data, err := json.Marshal(mcp.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	if tools.DefinitionName(def) != "create_thread" || !reflect.DeepEqual(map[string]any(def.GetFunction().Parameters), want) {
		t.Fatal("native and MCP create_thread schemas differ")
	}
	ctx := sessioncontext.WithSessionID(context.Background(), "parent")
	if _, err := tool.Execute(ctx, map[string]any{"prompt": "Inspect the checkout", "agent": "codex"}); err != nil {
		t.Fatal(err)
	}
	if service.spawned.ParentID != "parent" || service.spawned.ACPAgent != "codex" ||
		service.first.From != "parent" || service.first.Text != "Inspect the checkout" {
		t.Fatalf("lost caller or prompt: %#v %#v", service.spawned, service.first)
	}
}
