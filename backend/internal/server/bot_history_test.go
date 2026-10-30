package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBotHistoryPagesVisibleEventsAcrossEqualTimestamps(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	var events []sessionevents.Event
	for i := range 75 {
		// All events have the same stored millisecond, including page boundaries.
		at := base.Add(time.Duration(i) * time.Microsecond)
		events = append(events,
			sessionevents.Event{Type: "acp_tool", At: at, Content: strings.Repeat("private tool payload", 1000)},
			sessionevents.Event{Type: "room_message", At: at, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Name: "Bot", Text: fmt.Sprint(i)}},
		)
	}
	if err := store.AppendSessionEvents(bot.ID, events...); err != nil {
		t.Fatal(err)
	}
	handler := sessionMessagesHandler(store, store, nil)
	type page struct {
		Events          []sessionevents.Event `json:"events"`
		HasEarlier      bool                  `json:"has_earlier"`
		BeforeEventSeq  int64                 `json:"before_event_seq"`
		HistoryRevision int64                 `json:"history_revision"`
		LatestEventSeq  int64                 `json:"latest_event_seq"`
	}
	read := func(query, platform string) page {
		t.Helper()
		req := httptest.NewRequest("GET", "/v1/sessions/"+bot.ID+"/messages"+query, nil)
		req.Header.Set("X-Jaz-Client-Platform", platform)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != 200 {
			t.Fatalf("status %d: %s", res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), "private tool payload") {
			t.Fatal("bot history returned private tool payloads")
		}
		var result page
		if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	latest := read("", "desktop")
	if len(latest.Events) != 14 || latest.Events[0].RoomMessage.Text != "61" || latest.Events[13].RoomMessage.Text != "74" || !latest.HasEarlier {
		t.Fatalf("latest page: %+v", latest)
	}
	mobile := read("", "ios")
	if len(mobile.Events) != len(latest.Events) || mobile.BeforeEventSeq != latest.BeforeEventSeq {
		t.Fatalf("mobile cursor differs: %+v", mobile)
	}
	if err := store.AppendSessionEvents(bot.ID, sessionevents.Event{
		Type: "room_message", At: base.Add(time.Second),
		RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Name: "Bot", Text: "new"},
	}); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	current := latest
	for {
		for _, event := range current.Events {
			if seen[event.Seq] || event.RoomMessage == nil || event.RoomMessage.Text == "new" {
				t.Fatalf("overlap, missing message or new event in older page: %+v", event)
			}
			seen[event.Seq] = true
		}
		if !current.HasEarlier {
			break
		}
		current = read(fmt.Sprintf("?before_event_seq=%d&history_revision=%d&turns=24", current.BeforeEventSeq, current.HistoryRevision), "ios")
		if current.LatestEventSeq != 151 {
			t.Fatalf("live replay cursor = %d", current.LatestEventSeq)
		}
	}
	if len(seen) != 75 {
		t.Fatalf("paged %d of 75 messages", len(seen))
	}
	if refreshed := read("", "desktop"); refreshed.Events[len(refreshed.Events)-1].RoomMessage.Text != "new" {
		t.Fatal("new message missing from latest page")
	}
}

func TestBotHistoryKeepsChatSurfacesAndWorkerTranscripts(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	handler := sessionMessagesHandler(store, store, nil)
	for _, source := range []string{storage.SourceBot, storage.SourceBotWorker, ""} {
		t.Run(source, func(t *testing.T) {
			thread, err := store.CreateSession(storage.CreateSession{SourceType: source})
			if err != nil {
				t.Fatal(err)
			}
			at := time.Now().UTC()
			if err := store.AppendMessageRecords(thread.ID,
				storage.Message{Role: "user", Content: "Show my work", FromUser: true, CreatedAt: at},
				storage.Message{Role: "assistant", Content: "Private narration", CreatedAt: at.Add(time.Millisecond)},
				storage.Message{Role: "user", Content: "Show my work", CreatedAt: at.Add(time.Millisecond)},
			); err != nil {
				t.Fatal(err)
			}
			events := []sessionevents.Event{
				{Type: "bot_activity", BotActivity: &sessionevents.BotActivityEvent{Kind: "message_received", Label: "Planner"}},
				{Type: "room_message", RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Name: "Bot", Text: "Ready"}},
				{Type: "artifact", Artifact: &sessionevents.ArtifactEvent{Title: "Map", WidgetCode: "<svg></svg>"}},
				{Type: "mcp_app", MCPApp: &sessionevents.MCPAppEvent{ServerID: "tasks", Tool: "show", Presented: true, Result: json.RawMessage(`{"content":[]}`)}},
				{Type: "permission_request", Permission: &sessionevents.ACPPermission{ID: "question", Title: "Which project?", Questions: []sessionevents.ACPQuestion{{ID: "project", Question: "Which project?"}}}},
				{Type: "permission_response", Permission: &sessionevents.ACPPermission{ID: "question", Answers: map[string][]string{"project": {"Prototype"}}}},
				{Type: "loop_created", LoopCreated: &sessionevents.LoopCreatedEvent{LoopID: "review", LoopName: "Review"}},
				{Type: "agent_switch", Content: "Changed agent"},
				{Type: "acp", ACP: &sessionevents.ACPEvent{ID: thread.ID, SuggestedPrompt: "Review the plan"}},
				{Type: "acp_tool", Content: "Private tool payload"},
			}
			for i := range events {
				events[i].At = at.Add(time.Duration(i+2) * time.Millisecond)
			}
			if err := store.AppendSessionEvents(thread.ID, events...); err != nil {
				t.Fatal(err)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "/v1/sessions/"+thread.ID+"/messages", nil))
			if res.Code != 200 {
				t.Fatalf("status %d: %s", res.Code, res.Body.String())
			}
			var page struct {
				Messages []storage.Message     `json:"messages"`
				Events   []sessionevents.Event `json:"events"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			wantMessages, wantEvents := 3, 10
			if source == storage.SourceBot {
				wantMessages, wantEvents = 1, 9
			}
			if len(page.Messages) != wantMessages || len(page.Events) != wantEvents {
				t.Fatalf("messages=%d events=%d, want %d/%d", len(page.Messages), len(page.Events), wantMessages, wantEvents)
			}
			if !page.Messages[0].FromUser || page.Messages[0].Content != "Show my work" || source != storage.SourceBot && page.Messages[2].FromUser {
				t.Fatal("history lost typed human authorship")
			}
			if !slices.EqualFunc(page.Events, events[:wantEvents], func(a, b sessionevents.Event) bool {
				return a.Type == b.Type
			}) ||
				page.Events[2].Artifact.WidgetCode != "<svg></svg>" || page.Events[3].MCPApp.Tool != "show" || page.Events[4].Permission.Questions[0].Question != "Which project?" ||
				page.Events[5].Permission.Answers["project"][0] != "Prototype" || page.Events[6].LoopCreated.LoopID != "review" || page.Events[8].ACP.SuggestedPrompt != "Review the plan" {
				t.Fatal("chat surfaces lost their content")
			}
		})
	}
}

func TestHistoricalArtifactVisibilityIsIndependentOfItsHistoryPage(t *testing.T) {
	for _, test := range []struct {
		kind    string
		human   bool
		visible bool
		tied    bool
	}{
		{kind: "message_received", human: true},
		{kind: "group", human: true},
		{kind: "routine", visible: true},
		{kind: "file_changed", human: true, visible: true},
		{kind: "unknown"},
		{kind: "routine", tied: true},
	} {
		t.Run(fmt.Sprintf("%s-tied-%v", test.kind, test.tied), func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
			if err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC)
			var messages []storage.Message
			if test.human {
				messages = append(messages, storage.Message{Role: "user", Content: "Earlier human turn", FromUser: true, CreatedAt: at})
			}
			if test.kind != "file_changed" {
				messages = append(messages, storage.Message{Role: "user", Content: "Internal turn", CreatedAt: at.Add(time.Second)})
			}
			if err := store.AppendMessageRecords(bot.ID, messages...); err != nil {
				t.Fatal(err)
			}
			boundaryAt := at.Add(2 * time.Second)
			if test.tied {
				boundaryAt = at.Add(time.Second)
			}
			events := []sessionevents.Event{{Type: sessionevents.TypeBotActivity, At: boundaryAt, BotActivity: &sessionevents.BotActivityEvent{Kind: test.kind}}}
			for i := range 13 {
				events = append(events, sessionevents.Event{Type: "acp", At: at.Add(time.Duration(i+3) * time.Second), ACP: &sessionevents.ACPEvent{ID: fmt.Sprintf("%s-child-%d", bot.ID, i), ParentID: bot.ID, State: "running"}})
			}
			events = append(events, sessionevents.Event{Type: sessionevents.TypeArtifact, At: at.Add(16 * time.Second), Artifact: &sessionevents.ArtifactEvent{Title: "Historical map", WidgetCode: "<svg></svg>"}})
			if err := store.AppendSessionEvents(bot.ID, events...); err != nil {
				t.Fatal(err)
			}
			handler := sessionMessagesHandler(store, store, nil)
			type page struct {
				Events          []sessionevents.Event `json:"events"`
				BeforeEventSeq  int64                 `json:"before_event_seq"`
				HistoryRevision int64                 `json:"history_revision"`
			}
			read := func(query string) page {
				t.Helper()
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("GET", "/v1/sessions/"+bot.ID+"/messages"+query, nil))
				var result page
				if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil {
					t.Fatalf("history response: %d %s", response.Code, response.Body.String())
				}
				return result
			}
			latest := read("")
			if len(latest.Events) != 14 || latest.Events[0].Seq != 2 || latest.Events[13].Artifact == nil || latest.Events[13].Artifact.Presented != test.visible {
				t.Fatalf("visibility with clipped boundary: %+v", latest)
			}
			earlier := read(fmt.Sprintf("?before_event_seq=%d&history_revision=%d", latest.BeforeEventSeq, latest.HistoryRevision))
			full := read("?turns=24")
			merged := append(earlier.Events, latest.Events...)
			if len(merged) != 15 || !slices.EqualFunc(merged, full.Events, func(a, b sessionevents.Event) bool {
				return a.SessionID == b.SessionID && a.Seq == b.Seq && a.StorageContent() == b.StorageContent()
			}) {
				t.Fatal("merging earlier history changed artifact provenance")
			}
			raw, err := store.LoadSessionEvents(bot.ID)
			if err != nil || raw[14].Artifact.Presented || raw[14].Artifact.WidgetCode != "<svg></svg>" {
				t.Fatalf("history projection changed stored content: %+v, %v", raw, err)
			}
		})
	}
}
