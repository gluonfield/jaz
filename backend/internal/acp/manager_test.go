package acp_test

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestManagerSpawnsFakeACPAgentAndStoresSession(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: "go",
				Args:    []string{"run", "github.com/wins/acp-transport/example/fake-agent"},
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{
		ParentID: parent.ID,
		ACPAgent: "fake",
		Slug:     "fake-review",
		Message:  "say hello",
		Cwd:      t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle {
		t.Fatalf("state = %s, want %s; error=%s", job.State, acp.StateIdle, job.Error)
	}
	if job.Assistant != "hello from fake agent" {
		t.Fatalf("assistant = %q", job.Assistant)
	}

	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ParentID != parent.ID || session.Runtime != storage.RuntimeACP || session.RuntimeRef.SessionID != "fake-session" {
		t.Fatalf("unexpected session metadata %#v", session)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || provider.MessageContent(messages[1]) != "hello from fake agent" {
		t.Fatalf("unexpected messages %#v", messages)
	}
}
