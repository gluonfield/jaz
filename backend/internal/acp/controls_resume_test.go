package acp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestRestartPreservesEffortOwnership(t *testing.T) {
	for _, config := range []struct {
		agent    string
		explicit string
	}{{acp.AgentClaude, ""}, {acp.AgentClaude, "high"}, {acp.AgentCodex, ""}, {acp.AgentCodex, "high"}} {
		agent, explicit := config.agent, config.explicit
		t.Run(agent+"/explicit="+explicit, func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			requestLog := filepath.Join(t.TempDir(), "requests")
			options := `[{"id":"effort","name":"Effort","category":"thought_level","type":"select","currentValue":"high","options":[{"value":"high","name":"High"},{"value":"medium","name":"Medium"}]}]`
			env := map[string]string{
				"JAZ_FAKE_ACP_SESSION_OPTIONS":  options,
				"JAZ_FAKE_ACP_SET_CONFIG":       "1",
				"JAZ_FAKE_ACP_EXPECT_CONFIG_ID": "effort",
				"JAZ_FAKE_ACP_REQUEST_LOG":      requestLog,
			}
			manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), agent, env, "", explicit)
			t.Cleanup(manager.Close)
			spawned, err := manager.Spawn(t.Context(), acp.SpawnRequest{ACPAgent: agent, Slug: "effort-ownership"})
			if err != nil {
				t.Fatal(err)
			}
			env["JAZ_FAKE_ACP_SESSION_OPTIONS"] = strings.Replace(options, `"currentValue":"high"`, `"currentValue":"medium"`, 1)
			for range 2 {
				manager = restartNamedProcessTestManager(t, manager, store, agent, env)
				if err := os.WriteFile(requestLog, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := manager.Send(context.Background(), acp.SendRequest{Session: spawned.SessionID, Message: "continue", Completion: acp.CompletionInline}); err != nil {
					t.Fatal(err)
				}
				if job, err := manager.Wait(t.Context(), acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateIdle {
					t.Fatalf("resumed turn = %#v, %v", job, err)
				}
				requests, err := os.ReadFile(requestLog)
				if err != nil {
					t.Fatal(err)
				}
				wantCalls := 0
				if explicit != "" {
					wantCalls = 1
				}
				if calls := strings.Count(string(requests), `"configId":"effort"`); calls != wantCalls {
					t.Fatalf("effort writes after restart = %d, want %d; requests=%s", calls, wantCalls, requests)
				}
				saved, err := store.LoadSession(spawned.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				want := "medium"
				if explicit != "" {
					want = explicit
				}
				if saved.ReasoningEffort != want {
					t.Fatalf("reported effort = %s, want %s", saved.ReasoningEffort, want)
				}
			}
		})
	}
}

func TestRestartKeepsTheLevelAnAntigravityModelWasPickedWith(t *testing.T) {
	for _, pick := range []struct {
		name         string
		defaultModel string
		request      acp.SpawnRequest
	}{
		{name: "model and level from the picker", request: acp.SpawnRequest{Model: "neon", ReasoningEffort: "xhigh"}},
		{name: "saved catalogue id naming both", defaultModel: "neon-xhigh"},
	} {
		t.Run(pick.name, func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			env := map[string]string{
				"JAZ_FAKE_ACP_SESSION_OPTIONS":  `[{"id":"model","name":"Model","category":"model","type":"select","currentValue":"carbon","options":[{"value":"carbon","name":"Carbon"},{"value":"neon","name":"Neon"}]},{"id":"thought_level","name":"Thinking","category":"thought_level","type":"select","currentValue":"medium","options":[{"value":"medium","name":"Medium"},{"value":"xhigh","name":"XHigh"}]}]`,
				"JAZ_FAKE_ACP_SET_CONFIG":       "1",
				"JAZ_FAKE_ACP_MODEL_LEVELS":     "1",
				"JAZ_FAKE_ACP_EXPECT_CONFIG_ID": "thought_level",
			}
			manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), acp.AgentAntigravity, env, pick.defaultModel, "", func(cfg *acp.Config) {
				cfg.ModelCatalog = modelcatalog.NewService(nil)
			})
			t.Cleanup(manager.Close)
			pick.request.ACPAgent, pick.request.Slug = acp.AgentAntigravity, "antigravity-level"
			spawned, err := manager.Spawn(t.Context(), pick.request)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				manager = restartNamedProcessTestManager(t, manager, store, acp.AgentAntigravity, env)
				if _, err := manager.Send(context.Background(), acp.SendRequest{Session: spawned.SessionID, Message: "continue", Completion: acp.CompletionInline}); err != nil {
					t.Fatal(err)
				}
				if job, err := manager.Wait(t.Context(), acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateIdle {
					t.Fatalf("resumed turn = %#v, %v", job, err)
				}
				saved, err := store.LoadSession(spawned.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				if saved.Model != "neon" || saved.ReasoningEffort != "xhigh" {
					t.Fatalf("resumed on %s/%s, want neon/xhigh", saved.Model, saved.ReasoningEffort)
				}
			}
		})
	}
}
