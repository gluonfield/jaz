package sqlite

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestBotChatPagesHumanMessagesWithoutInternalTurnBoundaries(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	const text = "Task status: keep  two spaces, a\ttab and\na line break."
	var records []storage.Message
	for i := range 3 {
		records = append(records, storage.Message{Role: "user", Content: text, FromUser: true, CreatedAt: base.Add(time.Duration(i) * time.Hour)})
		for j := range 20 {
			records = append(records, storage.Message{Role: "user", Content: text, Blocks: []storage.Block{storage.TextBlock(text)}, CreatedAt: base.Add(time.Duration(i)*time.Hour + time.Duration(j+1)*time.Minute)})
		}
	}
	if err := store.AppendMessageRecords(bot.ID, records...); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(bot.ID,
		sessionevents.Event{Type: "bot_activity", At: base.Add(3 * time.Hour), BotActivity: &sessionevents.BotActivityEvent{Kind: "message_received", Label: "Worker", Text: text}},
		sessionevents.Event{Type: "room_message", At: base.Add(3 * time.Hour), RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Public delivery"}},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	request := storage.TranscriptPageRequest{Turns: 2, ChatOnly: true}
	latest, err := store.LoadTranscriptPage(t.Context(), bot.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest.Messages) != 2 || latest.Messages[0].Seq != 22 || latest.Messages[1].Seq != 43 || latest.BeforeMessageSeq != 22 || !latest.HasEarlier {
		t.Fatalf("chat page used internal turn boundaries: %+v", latest)
	}
	for _, message := range latest.Messages {
		if !message.FromUser || message.Content != text {
			t.Fatalf("lost authorship or whitespace: %+v", message)
		}
	}
	if len(latest.Events) != 2 || latest.Events[0].BotActivity.Text != text || latest.Events[1].RoomMessage.Text != "Public delivery" {
		t.Fatalf("lost activity or public delivery: %+v", latest.Events)
	}
	request.BeforeMessageSeq = latest.BeforeMessageSeq
	request.HistoryRevision = latest.HistoryRevision
	earlier, err := store.LoadTranscriptPage(t.Context(), bot.ID, request)
	if err != nil || len(earlier.Messages) != 1 || earlier.Messages[0].Seq != 1 || !earlier.Messages[0].FromUser || earlier.HasEarlier {
		t.Fatalf("earlier chat page = %+v, %v", earlier, err)
	}
	full, err := store.LoadMessageRecords(bot.ID)
	if err != nil || len(full) != len(records) {
		t.Fatalf("internal transcript lost: %d records, %v", len(full), err)
	}
	for i, message := range full {
		if message.Content != text || message.FromUser != records[i].FromUser {
			t.Fatalf("transcript record %d changed: %+v", i, message)
		}
	}
}

func TestBotChatPageBudgetCountsConversationEvents(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	var events []sessionevents.Event
	for range 20 {
		events = append(events, sessionevents.Event{Type: "room_message", At: at, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Public reply"}})
	}
	for i := range 300 {
		events = append(events,
			sessionevents.Event{Type: "acp", At: at, ACP: &sessionevents.ACPEvent{ID: fmt.Sprint(i), ParentID: bot.ID, State: "idle"}},
			sessionevents.Event{Type: "bot_activity", At: at, BotActivity: &sessionevents.BotActivityEvent{Kind: "message_received", Label: "Worker"}},
		)
	}
	if err := store.AppendSessionEvents(bot.ID, events...); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadSessionEvents(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := storage.TranscriptPageRequest{Turns: 14, ChatOnly: true}
	var merged []sessionevents.Event
	for {
		page, err := store.LoadTranscriptPage(t.Context(), bot.ID, request)
		if err != nil {
			t.Fatal(err)
		}
		if request.HistoryRevision == 0 && page.HistoryTargetEventSeq != 7 {
			t.Fatal("initial conversation target did not name the oldest of the newest14 replies")
		}
		if len(page.Events) == 0 || len(page.Events) > transcriptEventPageRows || page.LatestEventSeq != 620 {
			t.Fatal("conversation page lost its row bound or replay cursor")
		}
		merged = append(page.Events, merged...)
		if !page.HasEarlier {
			break
		}
		if page.BeforeEventSeq == 0 || request.BeforeEventSeq > 0 && page.BeforeEventSeq >= request.BeforeEventSeq {
			t.Fatal("conversation cursor did not advance")
		}
		request.BeforeEventSeq = page.BeforeEventSeq
		request.HistoryRevision = page.HistoryRevision
	}
	if !reflect.DeepEqual(merged, stored) {
		t.Fatal("chunked conversation lost, duplicated, reordered or changed event context")
	}
	after, err := store.LoadSessionEvents(bot.ID)
	if err != nil || !reflect.DeepEqual(after, stored) {
		t.Fatal("reading conversation pages changed stored history")
	}
}

func TestBotChatByteBudgetAdvancesThroughAuxiliaryHistory(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	events := []sessionevents.Event{{Type: "room_message", RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Older reply"}}}
	for i := range 40 {
		events = append(events, sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{ID: fmt.Sprint(i), ParentID: bot.ID, State: "failed", Error: strings.Repeat("x", 512<<10)}})
	}
	if err := store.AppendSessionEvents(bot.ID, events...); err != nil {
		t.Fatal(err)
	}
	request := storage.TranscriptPageRequest{Turns: 14, ChatOnly: true}
	seen := make(map[int64]bool)
	for {
		page, err := store.LoadTranscriptPage(t.Context(), bot.ID, request)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Events) == 0 || page.HasEarlier && (page.BeforeEventSeq == 0 || request.BeforeEventSeq > 0 && page.BeforeEventSeq >= request.BeforeEventSeq) {
			t.Fatal("byte-limited history did not advance")
		}
		bytes := 0
		for _, event := range page.Events {
			if seen[event.Seq] {
				t.Fatal("byte-limited history duplicated a record")
			}
			seen[event.Seq] = true
			bytes += len(event.StorageContent())
			if event.ACP != nil {
				if len(event.ACP.Error) != 512<<10 {
					t.Fatal("auxiliary status payload changed")
				}
				raw, err := json.Marshal(event.ACP)
				if err != nil {
					t.Fatal(err)
				}
				bytes += len(raw)
			}
		}
		if bytes > storage.MaxTranscriptEventBytes {
			t.Fatal("bot history exceeded its byte budget")
		}
		if !page.HasEarlier {
			break
		}
		request.BeforeEventSeq = page.BeforeEventSeq
		request.HistoryRevision = page.HistoryRevision
	}
	if len(seen) != len(events) || !seen[1] {
		t.Fatal("byte-limited history dropped records")
	}
}

func TestBotChatWithoutConversationHasNoHydrationTarget(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	var events []sessionevents.Event
	for i := range 1000 {
		events = append(events, sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{ID: fmt.Sprint(i), ParentID: bot.ID, State: "idle"}})
	}
	if err := store.AppendSessionEvents(bot.ID, events...); err != nil {
		t.Fatal(err)
	}
	page, err := store.LoadTranscriptPage(t.Context(), bot.ID, storage.TranscriptPageRequest{Turns: 14, ChatOnly: true})
	if err != nil || page.HistoryTargetEventSeq != 0 || !page.HasEarlier || len(page.Events) != transcriptEventPageRows {
		t.Fatalf("metadata-only initial page = target%d rows%d earlier%v, %v", page.HistoryTargetEventSeq, len(page.Events), page.HasEarlier, err)
	}
}
