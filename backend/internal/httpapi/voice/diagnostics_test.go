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

func TestDiagnosticsPreserveSeparateClientAndProviderClocks(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "voice-diagnostics"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(nil, live.NewTranscript(store, sessionevents.New()))
	record := live.Diagnostic{ID: "receipt", CallID: "call", Stage: "append_ack", RequestID: "delegation", EventID: "frame", SourceSeq: 7, At: time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond), ElapsedMS: 321}
	start, end := 1234.0, 2345.0
	record.ProviderStartMS, record.ProviderEndMS = &start, &end
	for _, valid := range []bool{true, false} {
		input := []live.Diagnostic{record}
		if !valid {
			input[0].CallID = ""
		}
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/diagnostics", bytes.NewReader(data))
		request.SetPathValue("session", session.ID)
		response := httptest.NewRecorder()
		handler.Diagnostics(response, request)
		want := http.StatusNoContent
		if !valid {
			want = http.StatusBadRequest
		}
		if response.Code != want {
			t.Fatalf("diagnostic HTTP %d, want %d: %s", response.Code, want, response.Body.String())
		}
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil || len(events) != 1 || events[0].Type != "voice_diagnostic" || events[0].Voice != nil || !events[0].At.After(record.At) {
		t.Fatalf("diagnostic event = %#v, %v", events, err)
	}
	var got live.Diagnostic
	if err := json.Unmarshal([]byte(events[0].Content), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != record.ID || got.CallID != record.CallID || got.RequestID != record.RequestID || got.EventID != record.EventID || got.SourceSeq != record.SourceSeq || !got.At.Equal(record.At) || got.ElapsedMS != 321 || *got.ProviderStartMS != start || *got.ProviderEndMS != end {
		t.Fatalf("diagnostic clocks/identity changed: %#v", got)
	}
}
