package sessions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
)

func TestPageRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/messages?before_message_seq=40&before_event_seq=90&history_revision=7&turns=24", nil)
	got, err := pageRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	want := storage.TranscriptPageRequest{BeforeMessageSeq: 40, BeforeEventSeq: 90, HistoryRevision: 7, Turns: 24}
	if got != want {
		t.Fatalf("request = %#v, want %#v", got, want)
	}
}

func TestPageRequestRejectsInvalidCursors(t *testing.T) {
	for _, query := range []string{
		"before_message_seq=-1",
		"before_message_seq=1",
		"before_event_seq=nope",
		"turns=0",
		"turns=49",
	} {
		req := httptest.NewRequest(http.MethodGet, "/messages?"+query, nil)
		if _, err := pageRequest(req); err == nil {
			t.Fatalf("accepted %q", query)
		}
	}
}

type progressLiveReader struct{ job acp.Job }

func (r progressLiveReader) HydrationJobs([]string) map[string]acp.HydrationView {
	return map[string]acp.HydrationView{r.job.ID: acp.HydrationViewFromJob(r.job)}
}

func TestBotMessagesRetainSafeToolProgressThroughChatOnlyHydration(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "bot-progress", Runtime: storage.RuntimeACP, SourceType: storage.SourceBot, SourceID: "bot"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	tool := sessionevents.ACPToolCall{
		ID: "tool-1", Kind: "execute", Status: "in_progress", StartedAt: now, UpdatedAt: now,
		Title: "secret-title", ToolName: "secret-tool-name",
		RawInput:  map[string]any{"command": "curl -u user:secret-password https://example.test"},
		RawOutput: json.RawMessage(`"` + strings.Repeat("secret-output", 100000) + `"`),
		Content:   []sessionevents.ACPToolContent{{Type: "text", Text: "secret-content"}},
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: "acp_tool", At: now, ACP: &sessionevents.ACPEvent{ID: session.ID, State: acp.StateRunning, ToolCalls: []sessionevents.ACPToolCall{tool}}}); err != nil {
		t.Fatal(err)
	}
	handler := NewMessagesHandler(transcript.NewService(store, progressLiveReader{acp.Job{ID: session.ID, State: acp.StateRunning, ToolCalls: []sessionevents.ACPToolCall{tool}, LastEventAt: now, LastToolAt: now}}))
	request := httptest.NewRequest(http.MethodGet, "/messages", nil)
	request.SetPathValue("session", session.ID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret-") || len(response.Body.Bytes()) > 10000 {
		t.Fatalf("private or oversized tool payload reached bot messages (%d bytes)", response.Body.Len())
	}
	body := messagesResponse{acpStateResponse: &acpStateResponse{}}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 0 || len(body.ACPToolCalls) != 0 {
		t.Fatal("ChatOnly bot response exposed full tool events")
	}
	if len(body.ACPToolProgress) != 1 {
		t.Fatalf("progress = %#v", body.ACPToolProgress)
	}
	progress := body.ACPToolProgress[0]
	if progress.ID != "tool-1" || progress.Kind != "execute" || progress.Status != "in_progress" || !progress.UpdatedAt.Equal(now) {
		t.Fatalf("progress metadata was lost: %#v", progress)
	}
}
