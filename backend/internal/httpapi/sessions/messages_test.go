package sessions

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
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
