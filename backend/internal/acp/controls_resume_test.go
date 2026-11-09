package acp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
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

func TestOpenCodeTurnUsesAdvertisedReasoning(t *testing.T) {
	for _, effort := range []string{"none", "high"} {
		t.Run(effort, func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			turnLog := filepath.Join(t.TempDir(), "turns")
			env := map[string]string{
				"JAZ_FAKE_ACP_SESSION_OPTIONS":  `[{"id":"thinking","category":"thought_level","type":"select","currentValue":"low","options":[{"value":"none"},{"value":"low"},{"value":"high"}]}]`,
				"JAZ_FAKE_ACP_EXPECT_CONFIG_ID": "thinking",
				"JAZ_FAKE_ACP_SET_CONFIG":       "1",
				"JAZ_FAKE_ACP_TURN_LOG":         turnLog,
			}
			manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), acp.AgentOpenCode, env, "", "")
			t.Cleanup(manager.Close)
			session, err := manager.CreateSession(t.Context(), acp.SpawnRequest{ACPAgent: acp.AgentOpenCode, Slug: "reasoning-bot", ReasoningEffort: effort})
			if err != nil {
				t.Fatal(err)
			}
			sendAndWait(t, t.Context(), manager, session.ID, "reasoning check")
			if turns := readFakeTurns(t, turnLog); len(turns) != 1 || turns[0].Effort != effort {
				t.Fatalf("turns = %v, want effort %q", turns, effort)
			}
		})
	}
}
