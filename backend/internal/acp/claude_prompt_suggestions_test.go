package acp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// Claude Code can spend another request predicting the next prompt.
// A thread a person writes to keeps the prediction, as does its relaunch; the
// threads nobody writes to start and resume without it.
func TestClaudeSessionsNobodyWritesToStartWithoutPromptSuggestions(t *testing.T) {
	for _, tc := range []struct {
		source  string
		watched bool
	}{
		{"", true},
		{storage.SourceBot, true},
		{storage.SourceBotWorker, false},
		{storage.SourceBotMember, false},
		{storage.SourceLoopRun, false},
		{storage.SourceMemorySearch, false},
		{storage.SourceMemorySource, false},
		{storage.SourceMemoryDream, false},
	} {
		opened := claudeSessionOpenOptions(t, tc.source)
		for _, method := range []string{"session/new", "session/resume"} {
			options, ok := opened[method]
			if !ok {
				t.Fatalf("source %q: no %s", tc.source, method)
			}
			value, set := options["promptSuggestions"]
			if set == tc.watched || set && value != false {
				t.Fatalf("source %q %s promptSuggestions = %#v (set %v), want suggestions %v", tc.source, method, value, set, tc.watched)
			}
		}
	}
}

// claudeSessionOpenOptions starts a Claude thread from source, restarts Jaz,
// and returns the _meta.claudeCode.options each session open sent.
func claudeSessionOpenOptions(t *testing.T, source string) map[string]map[string]any {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	requestLog := filepath.Join(root, "requests.jsonl")
	newManager := func() *acp.Manager {
		return newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentClaude, map[string]string{
			"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
		}, "", "")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	first := newManager()
	t.Cleanup(first.Close)
	spawn := acp.SpawnRequest{ACPAgent: acp.AgentClaude, Slug: "claude-" + source, SourceType: source}
	if source != "" {
		spawn.SourceID = "bot-of-" + source
	}
	spawned, err := first.Spawn(ctx, spawn)
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, first, spawned.SessionID, "first")
	first.Close()
	second := newManager()
	t.Cleanup(second.Close)
	sendAndWait(t, ctx, second, spawned.SessionID, "after restart")
	opened := map[string]map[string]any{}
	for _, request := range readFakeACPRequests(t, requestLog) {
		if request.Method != "session/new" && request.Method != "session/resume" {
			continue
		}
		var params struct {
			Meta struct {
				ClaudeCode struct {
					Options map[string]any `json:"options"`
				} `json:"claudeCode"`
			} `json:"_meta"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		opened[request.Method] = params.Meta.ClaudeCode.Options
	}
	return opened
}
