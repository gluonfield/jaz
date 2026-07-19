package acp

import (
	"context"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestProviderSubagentSessionInfoUpdatePublishesAndStores(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-subagents", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "session_info_update",
				"_meta": map[string]any{
					codexMetaKey: map[string]any{
						"providerSubagent": map[string]any{
							"provider":  "codex",
							"id":        "thread-1",
							"thread_id": "thread-1",
							"name":      "Newton",
							"task":      "next_rigidity",
							"status":    "running",
							"prompt":    "inspect the leak",
						},
					},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	select {
	case event := <-sub:
		if event.Type != sessionevents.TypeProviderSubagent || event.ProviderSubagent == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		if event.ProviderSubagent.ID != "thread-1" ||
			event.ProviderSubagent.Provider != AgentCodex ||
			event.ProviderSubagent.Name != "Newton" ||
			event.ProviderSubagent.Task != "next_rigidity" ||
			event.ProviderSubagent.Prompt != "inspect the leak" {
			t.Fatalf("provider subagent = %#v", event.ProviderSubagent)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ProviderSubagent == nil || stored[0].ProviderSubagent.ID != "thread-1" {
		t.Fatalf("stored events = %#v", stored)
	}
	wantKey := sessionevents.ProviderSubagentProjectionKey(session.ID, *stored[0].ProviderSubagent)
	if stored[0].ProjectionKey != wantKey {
		t.Fatalf("projection key = %q, want %q", stored[0].ProjectionKey, wantKey)
	}
	if got := manager.jobsByID[session.ID].Title; got != "" {
		t.Fatalf("subagent metadata changed title: %q", got)
	}
}

func TestProviderSubagentSparseUpdateAfterManagerRestartPublishesCompleteSnapshot(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "provider-restart", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	view := eventView{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex}
	first := NewManager(store, Config{}, nil)
	first.publishProviderSubagent(view, sessionevents.ProviderSubagentEvent{
		Provider: AgentCodex, ID: "worker", Name: "Newton", Task: "audit", Status: "running",
	})

	second := NewManager(store, Config{}, nil)
	second.Events = sessionevents.New()
	live := second.Events.Subscribe(t.Context(), session.ID)
	second.publishProviderSubagent(view, sessionevents.ProviderSubagentEvent{
		Provider: AgentCodex, ID: "worker", Status: "completed",
	})

	event := <-live
	if event.ProviderSubagent == nil || event.ProviderSubagent.Name != "Newton" || event.ProviderSubagent.Task != "audit" || event.ProviderSubagent.Status != "completed" {
		t.Fatalf("live provider snapshot = %#v", event.ProviderSubagent)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ProviderSubagent == nil || stored[0].ProviderSubagent.Name != "Newton" || stored[0].ProviderSubagent.Task != "audit" {
		t.Fatalf("stored provider snapshot = %#v", stored)
	}
}

func TestProviderSubagentMetadataDoesNotConsumeMessageChunk(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-subagent-message", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "acp-session"}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"messageId":     "subagent-root-visible",
				"content":       map[string]any{"type": "text", "text": "root-visible text"},
				"_meta": map[string]any{
					codexMetaKey: map[string]any{
						"providerSubagent": map[string]any{
							"provider": "codex",
							"id":       "thread-1",
							"status":   "running",
						},
					},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	seenSubagent := false
	seenMessage := false
	for range 2 {
		select {
		case event := <-sub:
			switch event.Type {
			case sessionevents.TypeProviderSubagent:
				seenSubagent = event.ProviderSubagent != nil && event.ProviderSubagent.ID == "thread-1"
			case "acp_message":
				seenMessage = event.Content == "root-visible text"
			default:
				t.Fatalf("unexpected event %#v", event)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if !seenSubagent || !seenMessage {
		t.Fatalf("seenSubagent=%v seenMessage=%v", seenSubagent, seenMessage)
	}
	if got := manager.jobsByID[session.ID].Assistant; got != "root-visible text" {
		t.Fatalf("assistant = %q", got)
	}
}

// The Agent tool call carries the subagent record (built by the adapter) and
// stays in the main transcript as the spawn marker.
func TestClaudeAgentToolCallPublishesRecordAndStaysInTranscript(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-agent-name", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentClaude, ACPSession: "acp-session"}, toolByID: map[string]sessionevents.ACPToolCall{}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    "task-parent",
				"title":         "Explore Electron main process",
				"status":        "pending",
				"_meta": map[string]any{
					"claudeCode": map[string]any{"toolName": "Agent"},
					"jaz": map[string]any{"providerSubagent": map[string]any{
						"provider": "claude",
						"id":       "task-parent",
						"status":   "running",
						"name":     "Explore Electron main process",
						"role":     "Explore",
						"prompt":   "Explore the Electron main process thoroughly.",
					}},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	select {
	case event := <-sub:
		if event.Type != sessionevents.TypeProviderSubagent || event.ProviderSubagent == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		sa := event.ProviderSubagent
		if sa.ID != "task-parent" || sa.Provider != AgentClaude || sa.Status != "running" ||
			sa.Name != "Explore Electron main process" || sa.Role != "Explore" ||
			sa.Prompt != "Explore the Electron main process thoroughly." {
			t.Fatalf("provider subagent = %#v", sa)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	if got := manager.jobsByID[session.ID].ToolCalls; len(got) != 1 || got[0].ID != "task-parent" {
		t.Fatalf("Agent tool call missing from main transcript: %#v", got)
	}
}

// A subagent's own nested tool call updates its activity and is dropped from the
// main transcript.
func TestClaudeSubagentChildPublishesActivityAndIsConsumed(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-subagent-child", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentClaude, ACPSession: "acp-session"}, toolByID: map[string]sessionevents.ACPToolCall{}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    "nested-tool",
				"title":         "Read file",
				"_meta": map[string]any{
					"claudeCode": map[string]any{"parentToolUseId": "task-parent"},
					"jaz": map[string]any{"providerSubagent": map[string]any{
						"provider": "claude",
						"id":       "task-parent",
						"status":   "running",
						"summary":  "Read file",
					}},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	select {
	case event := <-sub:
		if event.Type != sessionevents.TypeProviderSubagent || event.ProviderSubagent == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		if sa := event.ProviderSubagent; sa.ID != "task-parent" || sa.Status != "running" || sa.Summary != "Read file" {
			t.Fatalf("provider subagent = %#v", sa)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got := manager.jobsByID[session.ID].ToolCalls; len(got) != 0 {
		t.Fatalf("nested Claude tool leaked into main transcript: %#v", got)
	}
}

// A nested subagent tool call without a panel record (e.g. terminal output) is
// still kept out of the main transcript.
func TestClaudeSubagentInternalToolConsumedWithoutRecord(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-subagent-internal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentClaude, ACPSession: "acp-session"}, toolByID: map[string]sessionevents.ACPToolCall{}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
		Method: acpschema.ClientMethodSessionUpdate,
		Params: mustJSON(t, map[string]any{
			"sessionId": "acp-session",
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    "nested-tool",
				"_meta":         map[string]any{"claudeCode": map[string]any{"parentToolUseId": "task-parent"}},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	if got := manager.jobsByID[session.ID].ToolCalls; len(got) != 0 {
		t.Fatalf("nested Claude tool leaked into main transcript: %#v", got)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("expected no published events, got %#v", stored)
	}
}
