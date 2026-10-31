package acp_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/memorydream"
	"github.com/wins/jaz/backend/internal/memorysearch"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/usage"
)

func TestCodexMemoryUsesSelectedModelOrFails(t *testing.T) {
	for _, tc := range []struct {
		name, model, effort, available, wantModel, wantEffort string
	}{
		{name: "default", wantModel: "gpt-6-luna", wantEffort: "low"},
		{name: "explicit-luna", model: "gpt-6-luna", effort: "medium", wantModel: "gpt-6-luna", wantEffort: "medium"},
		{name: "explicit-other", model: "gpt-6.1-sol", effort: "high", wantModel: "gpt-6.1-sol", wantEffort: "high"},
		{name: "unavailable", available: "gpt-6-astra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, root := newTestStore(t)
			if _, err := settings.SaveMemorySettings(store, settings.MemorySettings{Enabled: true, Agent: acp.AgentCodex, Model: tc.model, ReasoningEffort: tc.effort}); err != nil {
				t.Fatal(err)
			}
			if _, err := settings.SaveAgentDefaults(store, settings.AgentDefaults{ACP: map[string]settings.ACPAgentDefaults{
				acp.AgentCodex: {Enabled: true, ModelProvider: provider.ProviderOpenAI, Model: "gpt-6-astra", ReasoningEffort: "xhigh"},
			}}); err != nil {
				t.Fatal(err)
			}
			available := tc.available
			if available == "" {
				available = "gpt-6-astra,gpt-6-luna,gpt-6.1-sol"
			}
			turnLog := filepath.Join(root, "turns.log")
			manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{
				"JAZ_FAKE_ACP_MODELS": available, "JAZ_FAKE_ACP_SET_CONFIG": "1", "JAZ_FAKE_ACP_TURN_LOG": turnLog,
			}, "gpt-6-astra", "xhigh")
			t.Cleanup(manager.Close)
			_, err := memorysearch.New(store, manager).SearchMemory(testContext(t), memoryservice.SearchRequest{Query: "Jaz"})
			turns := readFakeTurns(t, turnLog)
			if tc.available != "" {
				if err == nil || !strings.Contains(err.Error(), "gpt-6-luna") || len(turns) != 0 {
					t.Fatalf("unavailable memory model: error=%v turns=%v", err, turns)
				}
				_, err = memorydream.New(store, manager, sourcequeue.New(root)).RunDream(testContext(t), jazmem.DreamRequest{Root: root})
				if err == nil || !strings.Contains(err.Error(), "gpt-6-luna") || len(readFakeTurns(t, turnLog)) != 0 {
					t.Fatalf("unavailable dream model: %v", err)
				}
				return
			}
			if err != nil || len(turns) != 1 || turns[0].Model != tc.wantModel || turns[0].Effort != tc.wantEffort {
				t.Fatalf("memory model: error=%v turns=%v, want %s/%s", err, turns, tc.wantModel, tc.wantEffort)
			}
		})
	}
}

func TestMemoryWorkerPreservesProviderSelection(t *testing.T) {
	for _, tc := range []struct {
		provider, wantModel, wantEffort string
	}{
		{provider.ProviderOpenRouter, "z-ai/glm-5.2", "low"},
		{"internal", "internal-model", "high"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			store, root := newTestStore(t)
			if _, err := settings.SaveMemorySettings(store, settings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
				t.Fatal(err)
			}
			if _, err := settings.SaveAgentDefaults(store, settings.AgentDefaults{ACP: map[string]settings.ACPAgentDefaults{
				acp.AgentCodex: {Enabled: true, ModelProvider: tc.provider, Model: "internal-model", ReasoningEffort: "high"},
			}}); err != nil {
				t.Fatal(err)
			}
			manager := acp.NewManager(store, acp.Config{Root: root, Workspace: root, Agents: acp.AgentCatalog{
				acp.AgentCodex: {Local: true, ModelProvider: tc.provider, Model: "internal-model", ReasoningEffort: "high"},
			}}, nil)
			t.Cleanup(manager.Close)
			seen := make(chan acp.LocalAgentRequest, 1)
			manager.RegisterLocalAgent(acp.AgentCodex, localRunner{seen: seen})
			_, err := memorysearch.New(store, manager).SearchMemory(testContext(t), memoryservice.SearchRequest{Query: "Jaz"})
			if err != nil {
				t.Fatal(err)
			}
			thread := (<-seen).Session
			if thread.ModelProvider != tc.provider || thread.Model != tc.wantModel || thread.ReasoningEffort != tc.wantEffort {
				t.Fatalf("worker provider selection = %s/%s/%s", thread.ModelProvider, thread.Model, thread.ReasoningEffort)
			}
		})
	}
}

func TestMemoryWorkerUsageIsSeparateFromParent(t *testing.T) {
	for _, source := range []string{"", storage.SourceLoopRun} {
		t.Run(source, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			const agent = "local_helper"
			if _, err := settings.SaveMemorySettings(store, settings.MemorySettings{Enabled: true, Agent: agent}); err != nil {
				t.Fatal(err)
			}
			parent, err := store.CreateSession(storage.CreateSession{Slug: "parent", Runtime: storage.RuntimeACP, SourceType: source})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.AddUsage(parent.ID, storage.Usage{InputTokens: 1100, CachedInputTokens: 1000, OutputTokens: 40}); err != nil {
				t.Fatal(err)
			}
			manager := acp.NewManager(store, acp.Config{
				Root:      t.TempDir(),
				Workspace: t.TempDir(),
				Agents:    map[string]acp.AgentConfig{agent: {Local: true}},
			}, nil)
			t.Cleanup(manager.Close)
			seen := make(chan acp.LocalAgentRequest, 1)
			manager.RegisterLocalAgent(agent, localRunner{seen: seen})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			answer, err := memorysearch.New(store, manager).SearchMemory(ctx, memoryservice.SearchRequest{
				Query: "Jaz", Limit: 5, ParentID: parent.ID,
			})
			if err != nil || answer != "local reply" {
				t.Fatalf("answer = %q, error = %v", answer, err)
			}
			req := <-seen
			if req.Session.ParentID != parent.ID || req.Session.SourceType != storage.SourceMemorySearch || req.Session.RuntimeRef.MCPServerPolicy != acp.MCPServerPolicyMemorySearchWorker {
				t.Fatalf("worker session = %#v", req.Session)
			}
			if len(manager.List()) != 0 {
				t.Fatal("finished worker is still registered")
			}
			loaded, err := store.LoadSession(parent.ID)
			if err != nil || loaded.Usage.InputTokens != 1100 || loaded.Usage.CachedInputTokens != 1000 || loaded.Usage.OutputTokens != 40 {
				t.Fatalf("parent usage = %#v, %v", loaded.Usage, err)
			}
			days, err := usage.NewService(store).Daily(usage.DailyQuery{Days: 1, Location: time.UTC})
			if err != nil {
				t.Fatal(err)
			}
			categories := map[string]int64{}
			for _, category := range days[0].Categories {
				categories[category.Category] = category.Usage.InputOutputTokens()
			}
			parentCategory := source
			if parentCategory == "" {
				parentCategory = usage.CategoryChat
			}
			if len(categories) != 2 || categories[parentCategory] != 140 || categories[storage.SourceMemorySearch] != 8 || days[0].Usage.InputOutputTokens() != 148 {
				t.Fatalf("daily usage = %#v", days[0])
			}
		})
	}
}
