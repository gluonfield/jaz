package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// The session/update frames in testdata/claude-prompt-suggestions are what the
// Claude adapter v0.93.0-jaz.1 sent a client declaring Jaz's capabilities in a
// live Claude Code 2.1.300 session: two turns in one adapter process, then a
// fresh adapter process that resumes the session for a third and, holding no
// earlier suggestion, sends no clear. {"prompt":n} marks where the client sent a
// prompt and {"end":n} where it returned; command catalogs are left out.
func TestClaudeSuggestedPromptFromAdapterTraffic(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-suggestions", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentClaude, State: StateIdle}}
	manager.jobsByID[session.ID] = job
	shown := func() string {
		events, err := store.LoadSessionEvents(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range slices.Backward(events) {
			if event.ACP != nil && event.ACP.ID == session.ID {
				return event.ACP.SuggestedPrompt
			}
		}
		return ""
	}
	var got []string
	record := func(phase string) {
		got = append(got, fmt.Sprintf("%s %q", phase, shown()))
	}
	var done chan struct{}
	for _, process := range []string{"before-restart", "after-restart"} {
		for _, line := range readSuggestionCapture(t, process) {
			switch {
			case line.Prompt > 0:
				record("idle")
				done = job.startTurn(CompletionInline, false, false)
				if err := manager.publishACP(job.eventView()); err != nil {
					t.Fatal(err)
				}
				record("started")
			case line.End > 0:
				manager.completePromptCall(done, job, line.StopReason, nil)
			default:
				manager.jobsByACP[line.SessionID] = job
				if _, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
					Method: acpschema.ClientMethodSessionUpdate,
					Params: line.raw,
				}); rpcErr != nil {
					t.Fatal(rpcErr)
				}
			}
		}
	}
	record("idle")
	// Claude Code filtered out its first prediction, so the first turn ends with
	// none. The third turn starts on a fresh adapter that sends no clear.
	want := []string{
		`idle ""`, `started ""`,
		`idle ""`, `started ""`,
		`idle "run the test"`, `started ""`,
		`idle "commit greet.py and test_greet.py"`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("suggested prompts:\n got %q\nwant %q", got, want)
	}
}

type suggestionCaptureLine struct {
	Prompt     int    `json:"prompt"`
	End        int    `json:"end"`
	StopReason string `json:"stopReason"`
	SessionID  string `json:"sessionId"`
	raw        json.RawMessage
}

func readSuggestionCapture(t *testing.T, name string) []suggestionCaptureLine {
	t.Helper()
	file, err := os.Open(filepath.Join("testdata", "claude-prompt-suggestions", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var lines []suggestionCaptureLine
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var line suggestionCaptureLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		line.raw = slices.Clone(scanner.Bytes())
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}
