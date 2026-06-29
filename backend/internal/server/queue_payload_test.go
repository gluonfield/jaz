package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionQueueActionAppendsAttachmentOnlyPrompt(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queue-attachment-only"})
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, Workspace: t.TempDir()}
	handler := srv.Handler()
	attachment := uploadTestAttachment(t, handler, session.ID, "image.png", "image-bytes")

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/sessions/"+session.ID+"/queue",
		strings.NewReader(`{"op":"append","message":{"attachment_ids":["`+attachment.ID+`"]}}`),
	)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 1 || loaded.QueuedMessages[0].Text != "" {
		t.Fatalf("queue = %#v", loaded.QueuedMessages)
	}
	if got := loaded.QueuedMessages[0].AttachmentIDs; len(got) != 1 || got[0] != attachment.ID {
		t.Fatalf("queued attachment ids = %#v", got)
	}
}
