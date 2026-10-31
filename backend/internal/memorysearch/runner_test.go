package memorysearch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

var workerUsage = storage.Usage{InputTokens: 1200, CachedInputTokens: 900, OutputTokens: 60}

type testManager struct {
	store   *sqlitestore.Store
	spawn   acp.SpawnRequest
	message string
	id      string
	job     acp.Job
	err     error
}

func (m *testManager) RunWorker(_ context.Context, req acp.SpawnRequest, message string) (acp.Job, error) {
	m.spawn = req
	m.message = message
	session, err := m.store.CreateSession(storage.CreateSession{
		Slug:       req.Slug,
		ParentID:   req.ParentID,
		Runtime:    storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{Agent: req.ACPAgent},
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
	})
	if err != nil {
		return acp.Job{}, err
	}
	m.id = session.ID
	if err := m.store.AddUsage(session.ID, workerUsage); err != nil {
		return acp.Job{}, err
	}
	return m.job, m.err
}

func newTestRunner(t *testing.T) (*Runner, *testManager) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := settings.SaveMemorySettings(store, settings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	manager := &testManager{store: store, job: acp.Job{State: acp.StateIdle, Assistant: "Supported answer: projects/jaz"}}
	return New(store, manager), manager
}

func TestPublicMCPSearchUsesMemoryAgent(t *testing.T) {
	runner, manager := newTestRunner(t)
	service := memoryservice.New(nil, manager.store, nil, "")
	service.SetSearcher(runner)
	server := mcp.NewServer(&mcp.Implementation{Name: "memory-test", Version: "1"}, nil)
	service.AddMCPTools(server)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_search", Arguments: map[string]any{"query": "Jaz"}})
	if err != nil || result.IsError {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	if len(result.Content) != 1 || result.Content[0].(*mcp.TextContent).Text != manager.job.Assistant {
		t.Fatalf("result = %#v", result)
	}
	if manager.spawn.SourceType != storage.SourceMemorySearch || !strings.Contains(manager.message, "limit 10 and deep false") {
		t.Fatalf("search was not delegated: %#v", manager.spawn)
	}
}

func TestSearchUsesMemoryModelSettings(t *testing.T) {
	for _, tc := range []struct {
		config settings.MemorySettings
		model  string
		effort string
	}{
		{settings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}, provider.DefaultOpenAIModel, "low"},
		{settings.MemorySettings{Enabled: true, Agent: acp.AgentClaude, Model: "chosen-model", ReasoningEffort: "high"}, "chosen-model", "high"},
	} {
		runner, manager := newTestRunner(t)
		if _, err := settings.SaveMemorySettings(manager.store, tc.config); err != nil {
			t.Fatal(err)
		}
		if _, err := runner.SearchMemory(context.Background(), memoryservice.SearchRequest{Query: "Jaz"}); err != nil {
			t.Fatal(err)
		}
		if manager.spawn.ACPAgent != tc.config.Agent || manager.spawn.Model != tc.model || manager.spawn.ReasoningEffort != tc.effort {
			t.Fatalf("spawn = %#v", manager.spawn)
		}
	}
}

func TestSearchWorkerFailureRetainsUsage(t *testing.T) {
	runner, manager := newTestRunner(t)
	manager.err = errors.New("provider failed")
	_, err := runner.SearchMemory(context.Background(), memoryservice.SearchRequest{Query: "Jaz"})
	if !errors.Is(err, manager.err) {
		t.Fatalf("error = %v", err)
	}
	events, err := manager.store.UsageEvents(time.Time{}, time.Time{})
	if err != nil || len(events) != 1 || events[0].SourceType != storage.SourceMemorySearch || events[0].Usage.InputTokens != workerUsage.InputTokens {
		t.Fatalf("failure usage = %#v, %v", events, err)
	}
}

func TestSearchRejectsEmptyAnswer(t *testing.T) {
	runner, manager := newTestRunner(t)
	manager.job.Assistant = "  "
	if _, err := runner.SearchMemory(context.Background(), memoryservice.SearchRequest{Query: "Jaz"}); err == nil {
		t.Fatal("accepted empty answer")
	}
}

func TestSearchRequiresEnabledConfiguredMemoryAgent(t *testing.T) {
	for _, config := range []settings.MemorySettings{
		{Enabled: false, Agent: acp.AgentCodex},
		{Enabled: true},
		{Enabled: true, Agent: acp.AgentJaz},
	} {
		runner, manager := newTestRunner(t)
		if _, err := settings.SaveMemorySettings(manager.store, config); err != nil {
			t.Fatal(err)
		}
		if _, err := runner.SearchMemory(context.Background(), memoryservice.SearchRequest{Query: "Jaz"}); err == nil || manager.id != "" {
			t.Fatalf("error = %v, spawned = %q", err, manager.id)
		}
	}
}
