package acp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

var promptSuggestionSources = []string{
	"",
	storage.SourceBot,
	storage.SourceBotWorker,
	storage.SourceBotMember,
	storage.SourceLoopRun,
	storage.SourceMemorySearch,
	storage.SourceMemorySource,
	storage.SourceMemoryDream,
}

func TestClaudePromptSuggestionsOnlyInOrdinaryThreads(t *testing.T) {
	for _, source := range promptSuggestionSources {
		t.Run("source="+source, func(t *testing.T) {
			opened, _ := sessionPromptSuggestionOptions(t, acp.AgentClaude, source)
			for method, options := range opened {
				value, set := options["promptSuggestions"]
				if set != (source != "") || set && value != false {
					t.Errorf("%s promptSuggestions = %#v (set %v), want disabled %v", method, value, set, source != "")
				}
			}
		})
	}
}

func TestCodexPromptSuggestionsOnlyWhereSomeoneWrites(t *testing.T) {
	for _, source := range promptSuggestionSources {
		t.Run("source="+source, func(t *testing.T) {
			want := `"false"`
			if source == "" || source == storage.SourceBot {
				want = `""`
			}
			_, launches := sessionPromptSuggestionOptions(t, acp.AgentCodex, source)
			if len(launches) != 2 {
				t.Fatalf("launches = %q, want first launch and restart", launches)
			}
			for _, got := range launches {
				if got != want {
					t.Errorf("CODEX_ACP_PROMPT_SUGGESTIONS = %s, want %s", got, want)
				}
			}
		})
	}
}

func TestOtherAgentsKeepPromptSuggestionDefault(t *testing.T) {
	for _, agent := range []string{acp.AgentCodex, acp.AgentKimi, acp.AgentGrok, acp.AgentOpenCode, acp.AgentMuse, acp.AgentAntigravity} {
		t.Run(agent, func(t *testing.T) {
			opened, _ := sessionPromptSuggestionOptions(t, agent, storage.SourceBot)
			for method, options := range opened {
				if value, set := options["promptSuggestions"]; set {
					t.Fatalf("%s promptSuggestions = %#v, want provider default", method, value)
				}
			}
		})
	}
}

// sessionPromptSuggestionOptions opens a session of source, sends human and
// hidden turns, restarts the Manager and sends them again. It returns the
// Claude Code options each session open carried and the Codex ACP suggestion
// switch each agent process started with.
func sessionPromptSuggestionOptions(t *testing.T, agent, source string) (map[string]map[string]any, []string) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	requestLog := filepath.Join(root, "requests.jsonl")
	launchLog := filepath.Join(root, "suggestions.log")
	newManager := func() *acp.Manager {
		return newFakeNamedAgentManagerWithOptions(t, store, root, agent, map[string]string{
			"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_REQUEST_LOG": requestLog, "JAZ_FAKE_ACP_SUGGESTIONS_LOG": launchLog,
		}, "", "")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	first := newManager()
	t.Cleanup(first.Close)
	spawn := acp.SpawnRequest{ACPAgent: agent, Slug: agent + "-" + source, SourceType: source}
	if source != "" {
		spawn.SourceID = "bot-of-" + source
	}
	spawned, err := first.Spawn(ctx, spawn)
	if err != nil {
		t.Fatal(err)
	}
	sendTurns := func(manager *acp.Manager) {
		for _, hidden := range []bool{false, true} {
			var err error
			if hidden {
				_, err = manager.StartInternalTurn(ctx, acp.InternalTurnRequest{Session: spawned.SessionID, Message: "hidden turn"})
			} else {
				_, err = manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "human turn", FromUser: true, Completion: acp.CompletionInline})
			}
			if err != nil {
				t.Fatal(err)
			}
			job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
			if err != nil || job.State != acp.StateIdle {
				t.Fatalf("hidden %v turn = %#v, %v", hidden, job, err)
			}
		}
	}
	sendTurns(first)
	first.Close()
	second := newManager()
	t.Cleanup(second.Close)
	sendTurns(second)
	opened := map[string]map[string]any{}
	prompts := 0
	for _, request := range readFakeACPRequests(t, requestLog) {
		if request.Method == "session/prompt" {
			prompts++
		}
		if request.Method != "session/new" && request.Method != "session/resume" {
			continue
		}
		if _, duplicate := opened[request.Method]; duplicate {
			t.Fatalf("unexpected extra %s", request.Method)
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
	if len(opened) != 2 || prompts != 4 {
		t.Fatalf("got %d session opens and %d prompts, want one new and one resume with human and hidden turns each", len(opened), prompts)
	}
	launches, err := os.ReadFile(launchLog)
	if err != nil {
		t.Fatal(err)
	}
	return opened, strings.Fields(string(launches))
}
