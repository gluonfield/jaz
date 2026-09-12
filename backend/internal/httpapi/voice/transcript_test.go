package voice

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/voice/live"
)

func TestTranscriptPersistsSpeechAndPublishesReplacements(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "voice"})
	if err != nil {
		t.Fatal(err)
	}
	bus := sessionevents.New()
	updates := bus.Subscribe(t.Context(), session.ID)
	handler := NewHandler(nil, live.NewTranscript(store, bus))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/sessions/{session}/voice/transcript", handler.Transcript)
	spokenAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	message := sessionevents.VoiceMessage{ID: "utterance", CallID: "call", Role: "user", Text: "List", At: spokenAt}
	post := func(id string, messages []sessionevents.VoiceMessage) int {
		t.Helper()
		body, err := json.Marshal(messages)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/sessions/"+id+"/voice/transcript", bytes.NewReader(body)))
		return response.Code
	}
	for _, text := range []string{"List", "List the files.", "List the files."} {
		message.Text = text
		if status := post(session.ID, []sessionevents.VoiceMessage{message}); status != http.StatusNoContent {
			t.Fatalf("status = %d", status)
		}
		select {
		case event := <-updates:
			if event.Seq == 0 || event.Voice.Text != text || event.At.IsZero() {
				t.Fatalf("uncommitted event published: %#v", event)
			}
		case <-time.After(time.Second):
			t.Fatal("no transcript event published")
		}
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	events = sessionevents.CompactTranscript(events)
	if len(events) != 1 || events[0].Voice == nil || events[0].Voice.Text != "List the files." || !events[0].Voice.At.Equal(spokenAt) || events[0].Content != "" {
		t.Fatalf("reloaded transcript = %#v", events)
	}
	if status := post("missing-session", []sessionevents.VoiceMessage{message}); status != http.StatusNotFound {
		t.Fatalf("missing session status = %d", status)
	}
	message.Role = "system"
	if status := post(session.ID, []sessionevents.VoiceMessage{message}); status != http.StatusBadRequest {
		t.Fatalf("invalid role status = %d", status)
	}
}
