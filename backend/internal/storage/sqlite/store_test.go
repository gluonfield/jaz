package sqlite

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionsHaveStableUniqueSlugsAndRootListing(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first, err := store.CreateSession(storage.CreateSession{Slug: "Review ACP Transport", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSession(storage.CreateSession{Slug: "review-acp-transport", ParentID: first.ID, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "review-acp-transport" {
		t.Fatalf("unexpected first slug %q", first.Slug)
	}
	if second.Slug != "review-acp-transport-2" {
		t.Fatalf("unexpected second slug %q", second.Slug)
	}

	roots, err := store.ListSessions(storage.SessionFilter{RootOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].ID != first.ID {
		t.Fatalf("unexpected roots %#v", roots)
	}

	resolved, err := store.LoadSession(second.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != second.ID {
		t.Fatalf("resolved %s, want %s", resolved.ID, second.ID)
	}
}

func TestSessionQueuedMessagesRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	session.QueuedMessages = []string{"one prompt", "second prompt"}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(loaded.QueuedMessages, "|") != "one prompt|second prompt" {
		t.Fatalf("queued messages = %#v", loaded.QueuedMessages)
	}
}

func TestMCPServersCRUDRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.CreateMCPServer(mcpconfig.ServerInput{
		Name:              "Linear",
		URL:               "https://mcp.example.com/mcp",
		Enabled:           true,
		BearerTokenEnvVar: "LINEAR_TOKEN",
		Headers:           []mcpconfig.Header{{Name: "X-Team", Value: "platform"}},
		EnvHeaders:        []mcpconfig.EnvHeader{{Name: "X-Secret", EnvVar: "LINEAR_SECRET"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Transport != mcpconfig.TransportStreamableHTTP || !created.Enabled {
		t.Fatalf("created = %#v", created)
	}

	loaded, err := store.LoadMCPServer(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.BearerTokenEnvVar != "LINEAR_TOKEN" ||
		len(loaded.Headers) != 1 || loaded.Headers[0].Value != "platform" ||
		len(loaded.EnvHeaders) != 1 || loaded.EnvHeaders[0].EnvVar != "LINEAR_SECRET" {
		t.Fatalf("loaded = %#v", loaded)
	}

	updated, err := store.UpdateMCPServer(created.ID, mcpconfig.ServerInput{
		Name:    "Docs",
		URL:     "https://docs.example.com/mcp",
		Enabled: true,
		Headers: []mcpconfig.Header{{Name: "X-Docs", Value: "1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Docs" || updated.URL != "https://docs.example.com/mcp" ||
		updated.BearerTokenEnvVar != "" || len(updated.Headers) != 1 || updated.Headers[0].Name != "X-Docs" {
		t.Fatalf("updated = %#v", updated)
	}

	disabled, err := store.SetMCPServerEnabled(created.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled {
		t.Fatalf("disabled server still enabled: %#v", disabled)
	}

	servers, err := store.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].ID != created.ID {
		t.Fatalf("servers = %#v", servers)
	}
	if err := store.DeleteMCPServer(created.ID); err != nil {
		t.Fatal(err)
	}
	servers, err = store.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers after delete = %#v", servers)
	}
}

func TestSettingsCRUDRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.SaveSetting("agents", "defaults", []byte(`{"native":{"model":"test-model"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if created.Namespace != "agents" || created.Key != "defaults" || string(created.Value) != `{"native":{"model":"test-model"}}` {
		t.Fatalf("created = %#v", created)
	}

	loaded, err := store.LoadSetting("agents", "defaults")
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Value) != string(created.Value) {
		t.Fatalf("loaded = %#v", loaded)
	}

	settings, err := store.ListSettings("agents")
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 1 || settings[0].Key != "defaults" {
		t.Fatalf("settings = %#v", settings)
	}

	updated, err := store.SaveSetting("agents", "defaults", []byte(`{"native":{"model":"next-model"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) || updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Fatalf("timestamps were not preserved/advanced: created=%s/%s updated=%s/%s",
			created.CreatedAt, created.UpdatedAt, updated.CreatedAt, updated.UpdatedAt)
	}
	if err := store.DeleteSetting("agents", "defaults"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadSetting("agents", "defaults"); err == nil {
		t.Fatal("deleted setting still loads")
	}
}

func TestAddUsageStoresCachedTokensAndMirrors(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "usage"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.AddUsage(session.ID, storage.Usage{
		InputTokens:           100,
		CachedInputTokens:     64,
		OutputTokens:          25,
		ReasoningOutputTokens: 7,
		ContextWindowTokens:   400000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(session.ID, storage.Usage{
		InputTokens:       10,
		CachedInputTokens: 8,
		OutputTokens:      5,
		TotalTokens:       20,
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Missing totals derive from the disjoint components (100+64+25 = 189).
	if loaded.Usage.InputTokens != 110 || loaded.Usage.CachedInputTokens != 72 || loaded.Usage.OutputTokens != 30 ||
		loaded.Usage.ReasoningOutputTokens != 7 || loaded.Usage.TotalTokens != 209 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
	// Context reflects only the latest turn (10+8+5), never accumulates; the
	// window keeps its last reported value when later turns omit it.
	if loaded.Usage.ContextTokens != 23 || loaded.Usage.ContextWindowTokens != 400000 {
		t.Fatalf("context = %d / %d, want 23 / 400000", loaded.Usage.ContextTokens, loaded.Usage.ContextWindowTokens)
	}

	legacy, err := jsonstore.New(store.RootDir())
	if err != nil {
		t.Fatal(err)
	}
	mirrored, err := legacy.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mirrored.Usage != loaded.Usage {
		t.Fatalf("mirrored usage = %#v, want %#v", mirrored.Usage, loaded.Usage)
	}
}

func TestMigrateAddsUsageColumnsToLegacyThreads(t *testing.T) {
	root := t.TempDir()
	if err := makeLegacyDB(root); err != nil {
		t.Fatal(err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.CreateSession(storage.CreateSession{Slug: "usage"}); err != nil {
		t.Fatal(err)
	}
}

func TestSaveACPStateMirrorsStateAndUpdatesSessionStatus(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "acp", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveACPState(session.ID, storage.ACPState{State: "running", Assistant: "working"}); err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != "running" || state.Assistant != "working" {
		t.Fatalf("state = %#v", state)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusRunning {
		t.Fatalf("status = %q, want %q", loaded.Status, storage.StatusRunning)
	}

	if err := store.SaveACPState(session.ID, storage.ACPState{State: "failed", Error: "codex failed"}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusError {
		t.Fatalf("status = %q, want %q", loaded.Status, storage.StatusError)
	}
	if loaded.Error != "codex failed" {
		t.Fatalf("error = %q, want %q", loaded.Error, "codex failed")
	}

	if err := store.SaveACPState(session.ID, storage.ACPState{State: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusIdle {
		t.Fatalf("status = %q, want %q", loaded.Status, storage.StatusIdle)
	}
	if loaded.Error != "" {
		t.Fatalf("error = %q, want empty", loaded.Error)
	}
}

func TestSaveMessagesKeepsPendingToolCallUntilResultArrives(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "tools"})
	if err != nil {
		t.Fatal(err)
	}
	call := provider.FunctionToolCall("call_1", "mock", `{"value":"ok"}`)
	// Mid-turn snapshot: the call exists but its result hasn't arrived yet.
	if err := store.SaveMessages(session.ID, []provider.Message{
		provider.UserMessage("hello"),
		provider.AssistantMessage("", []provider.ToolCall{call}),
	}); err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	pending := records[1].Blocks[0]
	if pending.Type != "tool" || pending.Result != "" {
		t.Fatalf("pending call block = %#v, want empty result", pending)
	}
	loaded, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 || provider.MessageContent(loaded[2]) == "" {
		t.Fatalf("pending call should load with a placeholder tool result, got %#v", loaded)
	}
	createdAt := records[1].CreatedAt

	// The follow-up save fills in the result and keeps the original timestamp.
	if err := store.SaveMessages(session.ID, []provider.Message{
		provider.UserMessage("hello"),
		provider.AssistantMessage("", []provider.ToolCall{call}),
		provider.ToolMessage(`{"status":"ok"}`, "call_1"),
	}); err != nil {
		t.Fatal(err)
	}
	records, err = store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records after result save, want 2", len(records))
	}
	resolved := records[1].Blocks[0]
	if resolved.Result != `{"status":"ok"}` {
		t.Fatalf("tool result = %q, want stored result", resolved.Result)
	}
	if !records[1].CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at changed across saves: %v -> %v", createdAt, records[1].CreatedAt)
	}
}

func TestBackfillMissingThreadErrorsFromFailedToolResult(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "failed-parent"})
	if err != nil {
		t.Fatal(err)
	}
	call := provider.FunctionToolCall("call_1", "agent_send", `{"session":"codex"}`)
	if err := store.SaveMessages(session.ID, []provider.Message{
		provider.UserMessage("ask codex"),
		provider.AssistantMessage("", []provider.ToolCall{call}),
		provider.ToolMessage(`{"error":"context canceled","status":"error"}`, "call_1"),
	}); err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusError
	session.Error = ""
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	if err := store.backfillMissingThreadErrors(); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Error != "agent_send failed: context canceled" {
		t.Fatalf("error = %q", loaded.Error)
	}
}

func TestSaveMessagesWithReasoningPersistsBlocksWithoutReplay(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "reasoning"})
	if err != nil {
		t.Fatal(err)
	}
	err = store.SaveMessagesWithReasoning(session.ID, []provider.Message{
		provider.UserMessage("hello"),
		provider.AssistantMessage("done", nil),
	}, map[int]string{1: "thinking"})
	if err != nil {
		t.Fatal(err)
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[1].Reasoning != "thinking" {
		t.Fatalf("reasoning = %q, want thinking", records[1].Reasoning)
	}
	if len(records[1].Blocks) != 2 || records[1].Blocks[0].Type != blockReasoning || records[1].Blocks[1].Type != blockText {
		t.Fatalf("assistant blocks = %#v", records[1].Blocks)
	}

	replayed, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 2 || provider.MessageContent(replayed[1]) != "done" {
		t.Fatalf("unexpected replayed messages %#v", replayed)
	}
}

func TestToolCallAndResultPersistAsOneAssistantRecord(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "tools"})
	if err != nil {
		t.Fatal(err)
	}
	call := provider.FunctionToolCall("call_1", "mock", `{"value":"ok"}`)
	err = store.SaveMessages(session.ID, []provider.Message{
		provider.UserMessage("hello"),
		provider.AssistantMessage("", []provider.ToolCall{call}),
		provider.ToolMessage(`{"status":"completed"}`, "call_1"),
		provider.AssistantMessage("done", nil),
	})
	if err != nil {
		t.Fatal(err)
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3", len(records))
	}
	if len(records[1].Blocks) != 1 || records[1].Blocks[0].Type != blockTool {
		t.Fatalf("assistant tool record blocks = %#v", records[1].Blocks)
	}

	replayed, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 4 {
		t.Fatalf("replayed messages = %d, want 4", len(replayed))
	}
	if provider.MessageRole(replayed[2]) != "tool" || provider.MessageToolCallID(replayed[2]) != "call_1" {
		t.Fatalf("unexpected replayed tool result %#v", replayed[2])
	}

	legacy, err := jsonstore.New(store.RootDir())
	if err != nil {
		t.Fatal(err)
	}
	mirrored, err := legacy.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrored) != 4 {
		t.Fatalf("mirrored JSON messages = %d, want 4", len(mirrored))
	}
}

func TestToolMediaRefsPersistOnToolBlock(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "tool-media"})
	if err != nil {
		t.Fatal(err)
	}
	call := provider.FunctionToolCall("call_1", "view_image", `{"path":"image.png"}`)
	ref := media.Ref{
		Type:     media.TypeInputImage,
		Text:     "Image returned by view_image: image.png",
		BlobPath: "/tmp/blob",
		MimeType: "image/png",
		Size:     3,
		SHA256:   "abc",
		Detail:   "high",
		Filename: "image.png",
	}
	messages := []provider.Message{
		provider.UserMessage("look"),
		provider.AssistantMessage("", []provider.ToolCall{call}),
		provider.ToolMessage(`{"status":"ok"}`, "call_1"),
	}
	if err := store.SaveMessagesWithReasoningAndMedia(session.ID, messages, nil, map[string][]media.Ref{"call_1": []media.Ref{ref}}); err != nil {
		t.Fatal(err)
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || len(records[1].Blocks) != 1 {
		t.Fatalf("records = %#v", records)
	}
	if got := records[1].Blocks[0].MediaRefs; len(got) != 1 || got[0].BlobPath != ref.BlobPath {
		t.Fatalf("media refs = %#v, want persisted ref", got)
	}

	// Full-message saves that do not know about the sidecar must not erase it.
	if err := store.SaveMessages(session.ID, messages); err != nil {
		t.Fatal(err)
	}
	records, err = store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	refs := storage.MediaRefsByToolCall(records)
	if got := refs["call_1"]; len(got) != 1 || got[0].BlobPath != ref.BlobPath {
		t.Fatalf("sidecar refs after plain save = %#v", refs)
	}
}

func TestAppendMessagesPreservesExistingTimestamps(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "append-times"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessages(session.ID, provider.UserMessage("first")); err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	firstCreatedAt := records[0].CreatedAt

	time.Sleep(5 * time.Millisecond)
	if err := store.AppendMessages(session.ID, provider.UserMessage("second")); err != nil {
		t.Fatal(err)
	}
	records, err = store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if !records[0].CreatedAt.Equal(firstCreatedAt) {
		t.Fatalf("first timestamp changed from %s to %s", firstCreatedAt, records[0].CreatedAt)
	}
	if !records[1].CreatedAt.After(records[0].CreatedAt) {
		t.Fatalf("second timestamp %s should be after first %s", records[1].CreatedAt, records[0].CreatedAt)
	}
}

func TestSessionEventsPersistAndMirror(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "events"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: "acp_message", Content: "working"},
		sessionevents.Event{
			Type: "acp_tool",
			ACP: &sessionevents.ACPEvent{
				ID:        session.ID,
				ToolCalls: []sessionevents.ACPToolCall{{ID: "tool-1", Title: "Read file"}},
			},
		},
		sessionevents.Event{
			Type: "plan_update",
			Plan: &sessionevents.PlanEvent{
				Explanation: "Updated checklist",
				Plan:        []sessionevents.PlanEntry{{Content: "Run tests", Status: "pending"}},
			},
		},
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 || loaded[0].Seq != 1 || loaded[1].Seq != 2 || loaded[2].Seq != 3 {
		t.Fatalf("loaded events = %#v", loaded)
	}
	if loaded[1].ACP == nil || loaded[1].ACP.ToolCalls[0].Title != "Read file" {
		t.Fatalf("tool event = %#v", loaded[1])
	}
	if loaded[2].Plan == nil || loaded[2].Plan.Plan[0].Content != "Run tests" {
		t.Fatalf("plan event = %#v", loaded[2])
	}
	legacy, err := jsonstore.New(store.RootDir())
	if err != nil {
		t.Fatal(err)
	}
	mirrored, err := legacy.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrored) != 3 || mirrored[0].Content != "working" || mirrored[2].Plan == nil {
		t.Fatalf("mirrored events = %#v", mirrored)
	}
}

func TestImportLegacyJSONCopiesMissingSessions(t *testing.T) {
	root := t.TempDir()
	legacy, err := jsonstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := legacy.CreateSession(storage.CreateSession{Slug: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.SaveMessages(first.ID, []provider.Message{provider.UserMessage("first")}); err != nil {
		t.Fatal(err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	second, err := legacy.CreateSession(storage.CreateSession{Slug: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.SaveMessages(second.ID, []provider.Message{provider.UserMessage("second")}); err != nil {
		t.Fatal(err)
	}

	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.LoadMessages(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || provider.MessageContent(loaded[0]) != "second" {
		t.Fatalf("missing legacy session was not imported: %#v", loaded)
	}
}

func makeLegacyDB(root string) error {
	db, err := sql.Open("sqlite", filepath.Join(root, "jaz.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE threads (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT,
  parent_id TEXT,
  status TEXT NOT NULL DEFAULT 'idle',
  runtime TEXT NOT NULL DEFAULT 'native',
  acp_agent TEXT,
  acp_session_id TEXT,
  model_provider TEXT,
  model TEXT,
  reasoning_effort TEXT,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
)`)
	return err
}
