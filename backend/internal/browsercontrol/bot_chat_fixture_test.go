//go:build browserintegration

package browsercontrol_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
)

type botChatLive struct{}

func (botChatLive) HydrationJobs([]string) map[string]acp.HydrationView { return nil }

func mountBotChatFixture(t *testing.T, mux *http.ServeMux, store *sqlitestore.Store) {
	t.Helper()
	bot, err := store.CreateSession(storage.CreateSession{Slug: "paint-bot", SourceType: storage.SourceBot, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	var events []sessionevents.Event
	var messages []storage.Message
	for i := range 20 {
		when := at.Add(time.Duration(i) * time.Hour)
		messages = append(messages,
			storage.Message{Role: "user", FromUser: true, Content: fmt.Sprintf("Human request %02d", i), CreatedAt: when},
			storage.Message{Role: "user", Content: "Private worker coordination", CreatedAt: when},
		)
		events = append(events, sessionevents.Event{Type: "room_message", At: when.Add(time.Minute), RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Name: "Paint Bot", Text: fmt.Sprintf("Bot response %02d", i)}})
	}
	for i := range 300 {
		events = append(events,
			sessionevents.Event{Type: "acp", At: at.Add(21 * time.Hour), ACP: &sessionevents.ACPEvent{ID: fmt.Sprint("child-", i), ParentID: bot.ID, State: "idle"}},
			sessionevents.Event{Type: "bot_activity", At: at.Add(21 * time.Hour), BotActivity: &sessionevents.BotActivityEvent{Kind: "message_received", Label: "Worker"}},
		)
	}
	if err := store.AppendMessageRecords(bot.ID, messages...); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(bot.ID, events...); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var reads []int64
	handler := sessionsapi.NewMessagesHandler(transcript.NewService(store, botChatLive{}))
	earlier := make(chan struct{})
	var release sync.Once
	mux.HandleFunc("GET /v1/sessions/{session}/messages", func(w http.ResponseWriter, r *http.Request) {
		before, _ := strconv.ParseInt(r.URL.Query().Get("before_event_seq"), 10, 64)
		mu.Lock()
		reads = append(reads, before)
		mu.Unlock()
		if before > 0 && before <= 7 {
			select {
			case <-earlier:
			case <-r.Context().Done():
				return
			}
		}
		handler.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /exercise-bot-chat-reads", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(reads)
	})
	mux.HandleFunc("POST /exercise-bot-chat-release", func(w http.ResponseWriter, r *http.Request) {
		release.Do(func() { close(earlier) })
		fmt.Fprint(w, "{}")
	})
	mux.HandleFunc("POST /v1/sessions/{session}/seen", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(bot)
	})
	mux.HandleFunc("GET /exercise-bot-chat", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"id": bot.ID})
	})
	mux.HandleFunc("GET /v1/auth/check", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{}")
	})
	streams := make(map[chan sessionevents.Event]bool)
	mux.HandleFunc("GET /v1/sessions/{session}/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": ready\n\n")
		w.(http.Flusher).Flush()
		ch := make(chan sessionevents.Event, 8)
		mu.Lock()
		streams[ch] = true
		mu.Unlock()
		defer func() {
			mu.Lock()
			delete(streams, ch)
			mu.Unlock()
		}()
		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-ch:
				data, _ := json.Marshal(event)
				fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.Seq, data)
				w.(http.Flusher).Flush()
			}
		}
	})
	mux.HandleFunc("POST /exercise-bot-chat/{text}", func(w http.ResponseWriter, r *http.Request) {
		event, err := store.DeliverMessage(sessionevents.Event{SessionID: bot.ID, Type: "room_message", At: time.Now(), RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Name: "Paint Bot", Text: r.PathValue("text")}}, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		for ch := range streams {
			ch <- event
		}
		mu.Unlock()
		json.NewEncoder(w).Encode(event)
	})
}
