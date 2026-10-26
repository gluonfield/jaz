package sqlite

import (
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
