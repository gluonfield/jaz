package acp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestSetModelAppliesBeforeAndAfterTheAgentStarts(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	requestLog := filepath.Join(t.TempDir(), "requests")
	turnLog := filepath.Join(t.TempDir(), "turns")
	env := map[string]string{
		"JAZ_FAKE_ACP_SESSION_OPTIONS": `[` +
			`{"id":"model","name":"Model","category":"model","type":"select","currentValue":"fake-default","options":[{"value":"fake-default","name":"Default"},{"value":"fake-small","name":"Small"},{"value":"fake-large","name":"Large"}]},` +
			`{"id":"reasoning_effort","name":"Effort","category":"thought_level","type":"select","currentValue":"low","options":[{"value":"low","name":"Low"},{"value":"medium","name":"Medium"},{"value":"high","name":"High"}]}]`,
		"JAZ_FAKE_ACP_SET_CONFIG":  "1",
		"JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
		"JAZ_FAKE_ACP_TURN_LOG":    turnLog,
	}
	manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), acp.AgentCodex, env, "", "")
	t.Cleanup(manager.Close)
	session, err := manager.CreateSession(t.Context(), acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "picky-bot"})
	if err != nil {
		t.Fatal(err)
	}
	configWrites := func() []string {
		raw, err := os.ReadFile(requestLog)
		if err != nil {
			t.Fatal(err)
		}
		var writes []string
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var request struct {
				Method string `json:"method"`
				Params struct {
					ConfigID string `json:"configId"`
					Value    string `json:"value"`
				} `json:"params"`
			}
			if json.Unmarshal([]byte(line), &request) == nil && request.Method == "session/set_config_option" {
				writes = append(writes, request.Params.ConfigID+"="+request.Params.Value)
			}
		}
		return writes
	}

	if err := manager.SetModel(t.Context(), session.ID, "fake-large", "", "high"); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Model != "fake-large" || stored.ReasoningEffort != "high" {
		t.Fatalf("stored pick = %q/%q", stored.Model, stored.ReasoningEffort)
	}
	if _, err := manager.Send(t.Context(), acp.SendRequest{Session: session.ID, Message: "hello", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	if job, err := manager.Wait(t.Context(), acp.WaitRequest{Session: session.ID, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateIdle {
		t.Fatalf("first turn = %#v, %v", job, err)
	}
	if turns := readFakeTurns(t, turnLog); !slices.Equal(turns, []fakeTurn{{Model: "fake-large", Effort: "high"}}) {
		t.Fatalf("first turn = %v, want one started on fake-large/high", turns)
	}

	if err := os.WriteFile(requestLog, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetModel(t.Context(), session.ID, "fake-small", "", "medium"); err != nil {
		t.Fatal(err)
	}
	if writes := configWrites(); !slices.Equal(writes, []string{"model=fake-small", "reasoning_effort=medium"}) {
		t.Fatalf("running agent config writes = %v", writes)
	}
	if stored, err = store.LoadSession(session.ID); err != nil || stored.Model != "fake-small" || stored.ReasoningEffort != "medium" {
		t.Fatalf("stored live pick = %q/%q, %v", stored.Model, stored.ReasoningEffort, err)
	}
}

func TestOpenCodeChangesProviderWithoutReplacingItsNativeSession(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{"ALPHA_KEY": "alpha-test-key", "TALOS_KEY": "talos-test-key"}); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "requests")
	configure := func(cfg *acp.Config) {
		cfg.Providers = map[string]provider.ModelProviderConfig{
			"alpha": {Type: "openai-compatible", BaseURL: "https://alpha.example.test/v1", APIKeyEnv: "ALPHA_KEY", Models: []string{"markoff/first"}},
			"talos": {Type: "openai-compatible", BaseURL: "https://talos.example.test/v1", APIKeyEnv: "TALOS_KEY", Models: []string{"markoff/second"}},
		}
		agent := cfg.Agents[acp.AgentOpenCode]
		agent.ProviderMode = acp.AgentProviderModeAgentDefaults
		agent.ModelProvider = "alpha"
		agent.ModelProviderCapability = provider.CapabilityChatCompletions
		cfg.Agents[acp.AgentOpenCode] = agent
	}
	env := map[string]string{"JAZ_FAKE_ACP_OVERLAY_OPTIONS": "1", "JAZ_FAKE_ACP_SET_CONFIG": "1", "JAZ_FAKE_ACP_REQUEST_LOG": log, "JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_REJECT_OVERLAY_MODEL": "alpha/markoff/rejected"}
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentOpenCode, env, "markoff/first", "", configure)
	t.Cleanup(manager.Close)
	session, err := manager.CreateSession(t.Context(), acp.SpawnRequest{ACPAgent: acp.AgentOpenCode, Slug: "provider-bot"})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, t.Context(), manager, session.ID, "remember this conversation")
	before, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.RuntimeRef.SessionID == "" {
		t.Fatal("native session was not created")
	}
	if _, err := manager.Send(t.Context(), acp.SendRequest{Session: session.ID, Message: "block until cancelled", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetModel(t.Context(), session.ID, "markoff/second", "talos", ""); err == nil || !strings.Contains(err.Error(), "wait for") {
		t.Fatalf("active provider switch was not refused: %v", err)
	}
	busy, err := store.LoadSession(session.ID)
	if err != nil || busy.ModelProvider != before.ModelProvider || busy.Model != before.Model || busy.RuntimeRef.SessionID != before.RuntimeRef.SessionID {
		t.Fatalf("refused switch changed the active session: %+v, %v", busy, err)
	}
	if _, err := manager.Cancel(t.Context(), session.ID); err != nil {
		t.Fatal(err)
	}
	for _, pick := range []struct{ provider, model string }{{"talos", "markoff/second"}, {"alpha", "markoff/first"}, {"talos", "markoff/second"}} {
		deadline := time.Now().Add(5 * time.Second)
		for {
			err := manager.SetModel(t.Context(), session.ID, pick.model, pick.provider, "")
			if err == nil {
				break
			}
			if !strings.Contains(err.Error(), "wait for agent work") || time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
		}
		after, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.RuntimeRef.SessionID != before.RuntimeRef.SessionID || after.ModelProvider != pick.provider || after.Model != pick.model {
			t.Fatalf("selection or native identity changed: %+v", after)
		}
	}
	if err := manager.SetModel(t.Context(), session.ID, "markoff/rejected", "alpha", ""); err == nil {
		t.Fatal("failed launch was accepted")
	}
	restored, err := store.LoadSession(session.ID)
	if err != nil || restored.ModelProvider != "talos" || restored.Model != "markoff/second" || restored.RuntimeRef.SessionID != before.RuntimeRef.SessionID {
		t.Fatalf("failed switch changed the selection or native identity: %+v, %v", restored, err)
	}
	manager.Close()
	manager = newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentOpenCode, env, "markoff/first", "", configure)
	t.Cleanup(manager.Close)
	if _, err := manager.Send(t.Context(), acp.SendRequest{Session: session.ID, Message: "continue the same conversation", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	after, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ModelProvider != "talos" || after.Model != "markoff/second" || after.RuntimeRef.SessionID != before.RuntimeRef.SessionID {
		t.Fatalf("selection lost after restart: %+v", after)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), `"method":"session/new"`) != 1 {
		t.Fatalf("a provider change replaced the conversation: %s", raw)
	}
}

func TestSetModelKeepsOpenRouterForOpaqueModelID(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configure := func(cfg *acp.Config) {
		cfg.Providers = map[string]provider.ModelProviderConfig{
			"openrouter": {Type: "openai-compatible", BaseURL: "https://router.example.test/v1", Models: []string{"other/model"}},
			"openai":     {Type: "openai-compatible", BaseURL: "https://openai.example.test/v1", Models: []string{"gpt-5.5"}},
		}
		agent := cfg.Agents[acp.AgentOpenCode]
		agent.ProviderMode = acp.AgentProviderModeAgentDefaults
		agent.ModelProvider = "openrouter"
		cfg.Agents[acp.AgentOpenCode] = agent
	}
	manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), acp.AgentOpenCode, nil, "other/model", "", configure)
	t.Cleanup(manager.Close)
	session, err := manager.CreateSession(t.Context(), acp.SpawnRequest{ACPAgent: acp.AgentOpenCode, Slug: "router-bot"})
	if err != nil {
		t.Fatal(err)
	}
	for _, selectedProvider := range []string{"openrouter", ""} {
		if err := manager.SetModel(t.Context(), session.ID, "openai/gpt-5.5", selectedProvider, ""); err != nil {
			t.Fatal(err)
		}
		stored, err := store.LoadSession(session.ID)
		if err != nil || stored.ModelProvider != "openrouter" || stored.Model != "openai/gpt-5.5" {
			t.Fatalf("opaque model pick changed routing: %+v, %v", stored, err)
		}
	}
}
