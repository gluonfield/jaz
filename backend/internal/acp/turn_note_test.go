package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

// A turn note reaches the agent while the transcript keeps what the user
// wrote, and a compact, a command rather than a turn, goes out bare.
func TestATurnNoteReachesTheAgentAloneAndNeverACompact(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			acp.AgentCodex: {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1", "JAZ_FAKE_ACP_LOAD": "1", "JAZ_FAKE_ACP_REQUEST_LOG": requestLog},
			},
		},
		TurnNote: func(session storage.Session) (string, string) { return "Note for " + session.ID + ".\n\n", "" },
	}, log.New(io.Discard))
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "noted"})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, manager, spawned.SessionID, "hello")
	if _, err := manager.Compact(ctx, acp.CompactRequest{Session: spawned.SessionID}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second}); err != nil {
		t.Fatal(err)
	}

	prompts := agentPrompts(t, requestLog)
	want := []string{"Note for " + spawned.SessionID + ".\n\nhello", acp.CompactCommand}
	if len(prompts) != len(want) || prompts[0] != want[0] || prompts[1] != want[1] {
		t.Fatalf("prompts the agent got = %q, want %q", prompts, want)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || !strings.Contains(string(transcript), `"hello"`) || strings.Contains(string(transcript), "Note for") {
		t.Fatalf("transcript = %s, want only the user's hello", transcript)
	}
}
