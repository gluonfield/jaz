package acp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestProviderRetryStatusClearsWhenTheTurnMakesProgress(t *testing.T) {
	for _, progress := range []string{
		`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"done"}}`,
		`{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"thinking"}}`,
		`{"sessionUpdate":"tool_call","toolCallId":"tool-1","title":"work","status":"in_progress"}`,
		`{"sessionUpdate":"plan","entries":[{"content":"work","status":"in_progress","priority":"high"}]}`,
	} {
		var kind struct{ SessionUpdate string }
		if err := json.Unmarshal([]byte(progress), &kind); err != nil {
			t.Fatal(err)
		}
		t.Run(kind.SessionUpdate, func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			session, err := store.CreateSession(storage.CreateSession{
				Slug: "retry-status", Runtime: storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: AgentCodex, SessionID: "acp-session"},
			})
			if err != nil {
				t.Fatal(err)
			}
			turn := storage.Turn{StartedAt: time.Now(), Prompt: &storage.TurnPrompt{Message: "hi"}}
			if err := store.StartSessionTurn(session.ID, turn); err != nil {
				t.Fatal(err)
			}
			manager := NewManager(store, Config{}, nil)
			manager.Events = sessionevents.New()
			job := newIdleJob(session, AgentCodex, "acp-session", "", ModeState{})
			job.startStoredTurn(CompletionInline, turn)
			if err := manager.promptSending(job, "prompt-1"); err != nil {
				t.Fatal(err)
			}
			warning := map[string]any{
				"sessionUpdate": "session_info_update",
				"_meta": map[string]any{"jetbrains": map[string]any{"air": map[string]any{
					"sessionFailure": sessionFailure{ID: "retry-1", Category: "limit", Severity: "warning", Title: "429 Too Many Requests", Details: "Retrying in 10 minutes."},
				}}},
			}
			manager.applyJobUpdate(job, mustJSON(t, warning))
			want := "429 Too Many Requests\n\nRetrying in 10 minutes."
			if snapshot := job.Snapshot(); snapshot.State != StateRunning || snapshot.Error != want || snapshot.Assistant != "" {
				t.Fatalf("retry status = %+v", snapshot)
			}
			stored, err := store.LoadSession(session.ID)
			if err != nil || stored.Turn.Prompt.State != storage.PromptSending {
				t.Fatalf("retry warning changed prompt confirmation: %+v, %v", stored.Turn, err)
			}
			manager.applyJobUpdate(job, json.RawMessage(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":""}}`))
			meta := warning["_meta"].(map[string]any)
			meta["codex"] = map[string]any{"sideChat": map[string]any{"id": "side-1", "command": "side"}}
			meta["jetbrains"].(map[string]any)["air"].(map[string]any)["sessionFailure"] = sessionFailure{ID: "side-retry", Category: "limit", Severity: "warning", Title: "Side chat is retrying"}
			manager.applyJobUpdate(job, mustJSON(t, warning))
			manager.applyJobUpdate(job, json.RawMessage(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"side answer"},"_meta":{"codex":{"sideChat":{"id":"side-1","command":"side"}}}}`))
			manager.applyJobUpdate(job, json.RawMessage(`{"sessionUpdate":"plan","entries":[{"content":"side work","status":"in_progress","priority":"high"}],"_meta":{"codex":{"sideChat":{"id":"side-1","command":"side"}}}}`))
			manager.applyJobUpdate(job, json.RawMessage(`{"sessionUpdate":"session_info_update","_meta":{"jetbrains":{"air":{"sessionFailure":{"id":"notice-1","category":"unknown","severity":"warning","title":"Configuration notice"}}}}}`))
			manager.applyJobUpdate(job, json.RawMessage(`{"sessionUpdate":"plan","entries":[]}`))
			if snapshot := job.Snapshot(); snapshot.Error != want || len(snapshot.Plan) != 0 {
				t.Fatal("side chat, configuration notice or unchanged plan cleared the primary retry status")
			}
			if _, retry := job.turn.retryDelay(sessionFailure{Category: "limit", Actions: []string{"retry"}}); retry {
				t.Fatal("side chat progress allowed another retry of a failure the provider already retried")
			}
			manager.applyJobUpdate(job, json.RawMessage(progress))
			if snapshot := job.Snapshot(); snapshot.State != StateRunning || snapshot.Error != "" {
				t.Fatalf("status after progress = %+v", snapshot)
			}
			events, err := store.LoadSessionEvents(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			var statuses []string
			for _, event := range events {
				if event.Type == "acp" && event.ACP != nil {
					statuses = append(statuses, event.ACP.Error)
				}
			}
			if len(statuses) != 2 || statuses[0] != want || statuses[1] != "" {
				t.Fatalf("published retry statuses = %q", statuses)
			}
		})
	}
}

func TestRetryWarningPublishesWithUnchangedOrLockedTitle(t *testing.T) {
	for _, locked := range []bool{false, true} {
		name := "unchanged"
		if locked {
			name = "locked"
		}
		t.Run(name, func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			session, err := store.CreateSession(storage.CreateSession{Slug: "retry-title", Title: "Existing title", Runtime: storage.RuntimeACP})
			if err != nil {
				t.Fatal(err)
			}
			session.TitleLocked = locked
			if err := store.SaveSession(session); err != nil {
				t.Fatal(err)
			}
			manager := NewManager(store, Config{}, nil)
			manager.Events = sessionevents.New()
			job := newIdleJob(session, AgentCodex, "acp-session", "", ModeState{})
			job.startTurn(CompletionInline, false, false)
			title := session.Title
			if locked {
				title = "New runtime title"
			}
			sub := manager.Events.Subscribe(t.Context(), session.ID, "acp")
			manager.applyJobUpdate(job, mustJSON(t, map[string]any{
				"sessionUpdate": "session_info_update", "title": title,
				"_meta": map[string]any{"jetbrains": map[string]any{"air": map[string]any{
					"sessionFailure": sessionFailure{ID: "retry-1", Category: "limit", Severity: "warning", Title: "429 Too Many Requests"},
				}}},
			}))
			events, err := store.LoadSessionEvents(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 || events[0].ACP == nil || events[0].ACP.Error != "429 Too Many Requests" {
				t.Fatalf("persisted retry warning = %+v", events)
			}
			if job.Snapshot().Title != session.Title {
				t.Fatal("retry warning changed the session title")
			}
			select {
			case event := <-sub:
				if event.ACP == nil || event.ACP.Error != "429 Too Many Requests" {
					t.Fatalf("streamed retry warning = %+v", event)
				}
			default:
				t.Fatal("retry warning was not streamed")
			}
		})
	}
}
