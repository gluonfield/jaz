package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

type slowTool struct{ delay time.Duration }

func (s *slowTool) Definition() tools.Definition {
	return tools.Function("exec_command", "stub", true, map[string]any{"type": "object"})
}

func (s *slowTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	time.Sleep(s.delay)
	return tools.Result{Content: `{"status":"ok"}`}, nil
}

type requestRecorderProvider struct {
	requests []provider.Request
}

func (p *requestRecorderProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Message: provider.AssistantMessage("done", nil)}, nil
}

func (p *requestRecorderProvider) StreamComplete(_ context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.requests = append(p.requests, req)
	ch := make(chan provider.Event, 2)
	ch <- provider.Event{Type: provider.EventDelta, Delta: "done"}
	ch <- provider.Event{Type: provider.EventDone}
	close(ch)
	return ch, nil
}

func TestNativeTurnUsesStoredProviderModelAndReasoning(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:            "native-provider",
		Runtime:         storage.RuntimeNative,
		ModelProvider:   "openai",
		Model:           "gpt-test",
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorderProvider{}
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{Provider: recorder, MaxTurns: 1},
	}

	if status := srv.runNativeSession(context.Background(), session, "hello", false, nil); status != storage.StatusIdle {
		t.Fatalf("status = %s", status)
	}
	if len(recorder.requests) != 1 {
		t.Fatalf("requests = %#v", recorder.requests)
	}
	req := recorder.requests[0]
	if req.Provider != "openai" || req.Model != "gpt-test" || req.ReasoningEffort != "high" {
		t.Fatalf("unexpected provider request %#v", req)
	}
}

func TestCreateNativeSessionErrorsWhenStoredDefaultsAreIncomplete(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{
		Native: agentsettings.NativeAgentDefaults{Model: "gpt-test"},
		ACP:    map[string]agentsettings.ACPAgentDefaults{},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), "native provider is required") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestCreateNativeSessionErrorsWhenStoredProviderIsUnknown(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{
		Native: agentsettings.NativeAgentDefaults{
			ModelProvider: "missing",
			Model:         "gpt-test",
		},
		ACP: map[string]agentsettings.ACPAgentDefaults{},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	(&Server{Store: store}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), "unknown native provider") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestNativeStreamSendsAttachmentLinksAndKeepsTranscriptBlocks(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-attachments", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorderProvider{}
	handler := (&Server{
		Store:     store,
		Workspace: t.TempDir(),
		Agent:     &agent.Agent{Provider: recorder, MaxTurns: 1},
	}).Handler()
	attachment := uploadTestAttachment(t, handler, session.ID, "note.txt", "read me")

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"summarize","attachment_ids":["`+attachment.ID+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if len(recorder.requests) != 1 {
		t.Fatalf("requests = %#v", recorder.requests)
	}
	requestMessages := recorder.requests[0].Messages
	gotPrompt := provider.MessageContent(requestMessages[len(requestMessages)-1])
	if !strings.Contains(gotPrompt, "summarize\n\nAttachments:\n- note.txt: file://") {
		t.Fatalf("native prompt = %q", gotPrompt)
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Role != "user" || records[0].Content != "summarize" {
		t.Fatalf("records = %#v", records)
	}
	var found bool
	for _, block := range records[0].Blocks {
		if block.Type == "attachment" && block.ID == attachment.ID && block.URI == attachment.URI {
			found = true
		}
		if block.Type == "text" && strings.Contains(block.Text, "Attachments:") {
			t.Fatalf("transcript text leaked attachment prompt: %#v", block)
		}
	}
	if !found {
		t.Fatalf("attachment block not persisted: %#v", records[0].Blocks)
	}
}

// The transcript interleaves messages with session events by timestamp, so
// each row must be stamped when it was produced: the user message at turn
// start and each assistant round before its tools run.
func TestNativeStreamStampsRowsChronologically(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "native", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	delay := 60 * time.Millisecond
	srv := &Server{
		Store: store,
		Agent: &agent.Agent{
			Provider: mockprovider.New(),
			Tools:    tools.NewRegistry(&slowTool{delay: delay}),
			MaxTurns: 4,
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"run the mock"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want user + 2 assistant rounds: %#v", len(records), records)
	}
	if records[0].Role != "user" || records[1].Role != "assistant" || records[2].Role != "assistant" {
		t.Fatalf("unexpected roles: %s %s %s", records[0].Role, records[1].Role, records[2].Role)
	}
	for i := 1; i < len(records); i++ {
		if records[i].CreatedAt.Before(records[i-1].CreatedAt) {
			t.Fatalf("row %d stamped before row %d: %v >= %v", i+1, i, records[i-1].CreatedAt, records[i].CreatedAt)
		}
	}
	// The tool round is stamped before its tool executes; the final round after.
	gap := records[2].CreatedAt.Sub(records[1].CreatedAt)
	if gap < delay-10*time.Millisecond {
		t.Fatalf("tool round was not stamped before tool execution: gap %v, want >= %v", gap, delay)
	}
	var toolBlock *storage.Block
	for i := range records[1].Blocks {
		if records[1].Blocks[i].Type == "tool" {
			toolBlock = &records[1].Blocks[i]
		}
	}
	if toolBlock == nil || toolBlock.Result != `{"status":"ok"}` {
		t.Fatalf("tool block missing or unresolved: %#v", records[1].Blocks)
	}
}
