package acp

import (
	"context"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestContentChunksWithoutMessageIDPersist(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "missing-message-id-chunks", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: "test-agent", ACPSession: "acp-session"}}
	job.startTurn(CompletionInline, false, false)
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job

	updates := []map[string]any{
		{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "hello"}},
		{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "thinking"}},
		{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": " hard"}},
	}
	for _, update := range updates {
		_, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
			Method: acpschema.ClientMethodSessionUpdate,
			Params: mustJSON(t, map[string]any{
				"sessionId": "acp-session",
				"update":    update,
			}),
		})
		if rpcErr != nil {
			t.Fatal(rpcErr)
		}
	}
	manager.withACPTranscriptBarrier(job.eventView(), nil)

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored events = %#v", stored)
	}
	if stored[0].Type != sessionevents.TypeACPMessage ||
		stored[0].Content != "hello" ||
		stored[0].ACP.TextRunID == "" {
		t.Fatalf("stored message event = %#v", stored[0])
	}
	if stored[1].Type != sessionevents.TypeACPThought ||
		stored[1].ACP.Thought != "thinking hard" ||
		stored[1].ACP.TextRunID == "" ||
		stored[1].ACP.TextRunID == stored[0].ACP.TextRunID {
		t.Fatalf("stored thought event = %#v", stored[1])
	}
	if got := manager.jobsByID[session.ID].Assistant; got != "hello" {
		t.Fatalf("assistant = %q, want hello", got)
	}
}
