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
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
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

	if err := manager.SetModel(t.Context(), session.ID, "fake-large", "high"); err != nil {
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
	if err := manager.SetModel(t.Context(), session.ID, "fake-small", "medium"); err != nil {
		t.Fatal(err)
	}
	if writes := configWrites(); !slices.Equal(writes, []string{"model=fake-small", "reasoning_effort=medium"}) {
		t.Fatalf("running agent config writes = %v", writes)
	}
	if stored, err = store.LoadSession(session.ID); err != nil || stored.Model != "fake-small" || stored.ReasoningEffort != "medium" {
		t.Fatalf("stored live pick = %q/%q, %v", stored.Model, stored.ReasoningEffort, err)
	}
}
