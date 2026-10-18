package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// The session/update frames in testdata/claude-subagents are what the Claude
// adapter v0.93.0-jaz.1 sent to a client declaring Jaz's capabilities: live
// Claude Code 2.1.300 runs, and, for a failure, which a live run cannot produce
// on demand, the adapter's own SDK-shaped stream.
// Each file is one adapter process; a case with two files restarts Jaz and the
// adapter in between and resumes the same ACP session.
func TestClaudeSubagentRowsFromAdapterTraffic(t *testing.T) {
	type row struct{ name, role, status string }
	for _, tc := range []struct {
		name      string
		processes []string
		want      []row
	}{
		{"foreground", []string{"foreground"}, []row{{"Read token", "general-purpose", "completed"}}},
		{"background", []string{"background"}, []row{{"Read notes", "general-purpose", "completed"}}},
		{"nested", []string{"nested"}, []row{
			{"Coordinator", "general-purpose", "completed"},
			{"Inner reader", "general-purpose", "completed"},
		}},
		{"cancelled", []string{"cancelled"}, []row{{"Ping check", "general-purpose", "cancelled"}}},
		{"failed", []string{"failed"}, []row{{"Check the build", "general-purpose", "failed"}}},
		{"resumed after restart", []string{"restart-before", "restart-after"}, []row{{"Token keeper", "general-purpose", "completed"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			session, err := store.CreateSession(storage.CreateSession{Slug: "claude-subagents", Runtime: storage.RuntimeACP})
			if err != nil {
				t.Fatal(err)
			}
			launchPrompts := make(map[string]string)
			for _, process := range tc.processes {
				frames := readClaudeFrames(t, process)
				for _, frame := range frames {
					record := frame.Update.Meta.Jaz.ProviderSubagent
					if _, ok := launchPrompts[record.ID]; !ok && record.Prompt != "" {
						launchPrompts[record.ID] = record.Prompt
					}
				}
				manager := NewManager(store, Config{}, nil)
				manager.Events = sessionevents.New()
				job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentClaude, ACPSession: frames[0].SessionID}}
				manager.jobsByID[session.ID] = job
				manager.jobsByACP[frames[0].SessionID] = job
				for _, frame := range frames {
					if _, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
						Method: acpschema.ClientMethodSessionUpdate,
						Params: frame.raw,
					}); rpcErr != nil {
						t.Fatal(rpcErr)
					}
				}
				var transcript []string
				for _, call := range job.ToolCalls {
					transcript = append(transcript, call.ID)
				}
				for _, frame := range frames {
					if frame.ParentToolUseID() != "" && slices.Contains(transcript, frame.Update.ToolCallID) {
						t.Fatalf("%s: subagent tool call %s is in the main transcript", process, frame.Update.ToolCallID)
					}
					if frame.Update.SessionUpdate == "tool_call" && frame.ParentToolUseID() == "" && !slices.Contains(transcript, frame.Update.ToolCallID) {
						t.Fatalf("%s: tool call %s is missing from the main transcript", process, frame.Update.ToolCallID)
					}
				}
			}

			stored, err := store.LoadSessionEvents(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			var got []row
			for _, event := range stored {
				if subagent := event.ProviderSubagent; event.Type == sessionevents.TypeProviderSubagent && subagent != nil {
					got = append(got, row{subagent.Name, subagent.Role, string(subagent.Status)})
					if subagent.Prompt != launchPrompts[subagent.ID] {
						t.Fatalf("%s prompt = %q, want its launch prompt %q", subagent.Name, subagent.Prompt, launchPrompts[subagent.ID])
					}
				}
			}
			slices.SortFunc(got, func(a, b row) int { return strings.Compare(a.name, b.name) })
			if !slices.Equal(got, tc.want) {
				t.Fatalf("Subtasks rows = %v, want %v", got, tc.want)
			}
		})
	}
}

type claudeFrame struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		SessionUpdate string `json:"sessionUpdate"`
		ToolCallID    string `json:"toolCallId"`
		Meta          struct {
			ClaudeCode struct {
				ParentToolUseID string `json:"parentToolUseId"`
			} `json:"claudeCode"`
			Jaz struct {
				ProviderSubagent struct {
					ID     string `json:"id"`
					Prompt string `json:"prompt"`
				} `json:"providerSubagent"`
			} `json:"jaz"`
		} `json:"_meta"`
	} `json:"update"`
	raw json.RawMessage
}

func (f claudeFrame) ParentToolUseID() string {
	return f.Update.Meta.ClaudeCode.ParentToolUseID
}

func readClaudeFrames(t *testing.T, name string) []claudeFrame {
	t.Helper()
	file, err := os.Open(filepath.Join("testdata", "claude-subagents", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var frames []claudeFrame
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var frame claudeFrame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		frame.raw = slices.Clone(scanner.Bytes())
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 {
		t.Fatalf("%s has no frames", name)
	}
	return frames
}
