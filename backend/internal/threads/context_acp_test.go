package threads

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestReadThreadFindsDeliveredBotReplies(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	session, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(100, 0).UTC()
	if err := store.AppendMessageRecords(session.ID, storage.Message{
		Role: "user", FromUser: true, Content: "What should we improve?", CreatedAt: start,
	}); err != nil {
		t.Fatal(err)
	}
	replies := []string{"Start with recoverable sends.", "Also preserve legitimate follow-ups."}
	for i, reply := range replies {
		_, err := store.DeliverMessage(sessionevents.Event{
			SessionID: session.ID,
			Type:      sessionevents.TypeRoomMessage,
			At:        start.Add(time.Duration(i+1) * time.Second),
			RoomMessage: &sessionevents.RoomMessageEvent{
				Speaker: "bot", BotID: session.ID, Text: reply, Answers: start,
			},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: sessionevents.TypeBotActivity, At: start.Add(3 * time.Second),
		BotActivity: &sessionevents.BotActivityEvent{Kind: "message_sent", Text: "Internal note about recoverable sends."},
	}); err != nil {
		t.Fatal(err)
	}
	service := NewService(sqlitestore.NewSearchQueries(store), store)
	read := func(req ContextRequest) ContextResponse {
		t.Helper()
		req.Session = session.ID
		req.IncludeTools = IncludeToolsNone
		_, result, err := service.ContextMCP(t.Context(), nil, req)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	tail := read(ContextRequest{Limit: 2})
	var got []string
	for _, message := range tail.Messages {
		if message.Role != "assistant" {
			t.Fatalf("tail included a non-assistant message: %#v", message)
		}
		got = append(got, message.Text)
	}
	if !slices.Equal(got, replies) || !tail.HasMoreBefore || tail.Session.MessageCount != 3 {
		t.Fatalf("tail did not return both deliveries: %#v", tail)
	}
	query := read(ContextRequest{Query: "recoverable sends", Limit: 1, MaxTextChars: 10})
	if query.MatchCount != 1 || len(query.Messages) != 1 || !query.Messages[0].Matched ||
		!query.Messages[0].Truncated || query.Messages[0].Text != "Start with..." || query.Messages[0].Seq != tail.Messages[0].Seq {
		t.Fatalf("query did not find the delivered reply beyond the display limit: %#v", query)
	}
	page := read(ContextRequest{AfterSeq: query.Messages[0].Seq, Limit: 1})
	if len(page.Messages) != 1 || page.Messages[0].Text != replies[1] || page.HasMoreAfter {
		t.Fatalf("paging missed the follow-up: %#v", page)
	}
}

func TestThreadToolsReadAndSearchEventBackedAnswers(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	session, err := store.CreateSession(storage.CreateSession{
		Slug: "event-backed", Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: "codex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-time.Minute)
	if err := store.AppendMessageRecords(session.ID, storage.Message{Role: "user", Content: "What did you find?", CreatedAt: start}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: sessionevents.TypeACPThought, At: start.Add(time.Second), ACP: &sessionevents.ACPEvent{ID: session.ID, Thought: "privatethought"}},
		sessionevents.Event{Type: "acp_tool", At: start.Add(2 * time.Second), ACP: &sessionevents.ACPEvent{ID: session.ID, ToolCalls: []sessionevents.ACPToolCall{{ID: "check", ToolName: "exec", RawOutput: json.RawMessage(`"privatetool"`)}}}},
		sessionevents.Event{Type: sessionevents.TypeACPMessage, At: start.Add(3 * time.Second), Content: "childsecret", ACP: &sessionevents.ACPEvent{ID: "child"}},
		sessionevents.Event{Type: sessionevents.TypeACPMessage, At: start.Add(4 * time.Second), Content: "Warning: Falling back from WebSockets to HTTPS transport.", ACP: &sessionevents.ACPEvent{ID: session.ID}},
		sessionevents.Event{Type: sessionevents.TypeACPMessage, At: start.Add(5 * time.Second), Content: "A qua", ACP: &sessionevents.ACPEvent{ID: session.ID, TextRunID: "message:answer"}},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: sessionevents.TypeACPMessage, At: start.Add(6 * time.Second), Content: "rtz result.", ACP: &sessionevents.ACPEvent{ID: session.ID, TextRunID: "message:answer"}},
	); err != nil {
		t.Fatal(err)
	}
	service := NewService(sqlitestore.NewSearchQueries(store), store)
	t.Run("read", func(t *testing.T) {
		_, result, err := service.ContextMCP(t.Context(), nil, ContextRequest{Session: session.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Messages) != 3 || result.Messages[2].Text != "A quartz result." || result.ToolCounts["exec"] != 1 {
			t.Fatalf("missing agent answer or tool: %#v", result)
		}
		body, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		for _, hidden := range []string{"privatethought", "privatetool", "childsecret", "WebSockets"} {
			if strings.Contains(string(body), hidden) {
				t.Fatalf("read exposed %q", hidden)
			}
		}
		_, earlier, err := service.ContextMCP(t.Context(), nil, ContextRequest{Session: session.ID, BeforeSeq: result.Messages[2].Seq, Limit: 1})
		if err != nil || len(earlier.Messages) != 1 || len(earlier.Messages[0].Tools) != 1 {
			t.Fatalf("paging lost tool message: %#v, %v", earlier, err)
		}
		_, withoutTools, err := service.ContextMCP(t.Context(), nil, ContextRequest{
			Session: session.ID, BeforeSeq: result.Messages[2].Seq, IncludeTools: IncludeToolsNone,
		})
		if err != nil || len(withoutTools.Messages) != 1 || withoutTools.Messages[0].Role != "user" {
			t.Fatalf("tool visibility changed the paging boundary: %#v, %v", withoutTools, err)
		}
		_, query, err := service.ContextMCP(t.Context(), nil, ContextRequest{Session: session.ID, Query: "quartz", IncludeTools: IncludeToolsNone})
		if err != nil || query.MatchCount != 1 || query.Messages[0].Text != "A quartz result." || query.ToolCounts["exec"] != 1 {
			t.Fatalf("query lost answer or tool count: %#v, %v", query, err)
		}
	})
	t.Run("search", func(t *testing.T) {
		for {
			compaction, err := store.CompactNextSessionEvents(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if compaction.ThreadID == "" {
				break
			}
		}
		_, result, err := service.SearchMCP(t.Context(), nil, SearchRequest{Query: "quartz"})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Threads) != 1 || result.Threads[0].ThreadID != session.ID {
			t.Fatalf("missing event-backed search result: %#v", result)
		}
		for _, hidden := range []string{"privatethought", "privatetool", "childsecret", "WebSockets"} {
			_, result, err := service.SearchMCP(t.Context(), nil, SearchRequest{Query: hidden})
			if err != nil || len(result.Threads) != 0 {
				t.Fatalf("search exposed %q: %#v, %v", hidden, result, err)
			}
		}
	})
	t.Run("hidden warnings do not consume search limit", func(t *testing.T) {
		events := make([]sessionevents.Event, maxSearchCandidates)
		for i := range events {
			events[i] = sessionevents.Event{
				Type: sessionevents.TypeACPMessage, Content: "Warning: Falling back from WebSockets to HTTPS transport.",
				ACP: &sessionevents.ACPEvent{ID: session.ID},
			}
		}
		events = append(events, sessionevents.Event{
			Type: sessionevents.TypeACPMessage, Content: "WebSockets " + strings.Repeat("work ", 100),
			ACP: &sessionevents.ACPEvent{ID: session.ID},
		})
		if err := store.AppendSessionEvents(session.ID, events...); err != nil {
			t.Fatal(err)
		}
		_, result, err := service.SearchMCP(t.Context(), nil, SearchRequest{Query: "WebSockets", Limit: 1})
		if err != nil || len(result.Threads) != 1 || result.Threads[0].ThreadID != session.ID {
			t.Fatalf("hidden matches crowded out the visible answer: %#v, %v", result, err)
		}
	})
}
