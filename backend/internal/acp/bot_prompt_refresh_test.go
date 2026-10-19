package acp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// Claude Code resends the system prompt it recorded at a conversation's first
// request whenever the session is resumed, unless the resume asks it to
// render the prompt it is given. A bot keeps one conversation for good, so a
// restored bot asks; other threads keep the recorded prompt.
func TestRestoredClaudeBotRendersItsCurrentPrompt(t *testing.T) {
	for _, tc := range []struct {
		source string
		fresh  bool
	}{
		{storage.SourceBot, true},
		{"", false},
	} {
		prompt := restoredClaudeSystemPrompt(t, tc.source)
		if appended, _ := prompt["append"].(string); !strings.Contains(appended, "current prompt") {
			t.Fatalf("source %q restored without its prompt: %#v", tc.source, prompt)
		}
		snapshot, set := prompt["snapshot"]
		if set != tc.fresh || set && snapshot != false {
			t.Fatalf("source %q restored with snapshot %#v (set %v), want fresh render %v", tc.source, snapshot, set, tc.fresh)
		}
	}
}

// restoredClaudeSystemPrompt starts a Claude thread from source, restarts Jaz,
// and returns the _meta.systemPrompt the thread's resume sent.
func restoredClaudeSystemPrompt(t *testing.T, source string) map[string]any {
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
		}, "", "", func(cfg *acp.Config) {
			cfg.ResumePrompt = func(storage.Session) (promptmodule.Modules, error) {
				return promptmodule.New("current prompt"), nil
			}
		})
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	first := newManager()
	spawned, err := first.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentClaude, Slug: "claude-" + source, SourceType: source})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, first, spawned.SessionID, "first")
	first.Close()
	second := newManager()
	t.Cleanup(second.Close)
	sendAndWait(t, ctx, second, spawned.SessionID, "after restart")
	for _, request := range readFakeACPRequests(t, requestLog) {
		if request.Method != "session/resume" {
			continue
		}
		var params struct {
			Meta struct {
				SystemPrompt map[string]any `json:"systemPrompt"`
			} `json:"_meta"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		return params.Meta.SystemPrompt
	}
	t.Fatalf("source %q: the restarted manager never resumed the Claude session", source)
	return nil
}
