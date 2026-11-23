package acp_test

import (
	"bytes"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestBotRegistersSendsAndDownloadsOutboxFile(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	jaz.server.Workspace = root
	bot := jaz.newBot("Research")
	s, err := store.LoadSession(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.RuntimeRef.Cwd, ".jaz-outbox")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const name = "résumé.bin"
	data := []byte{0, 1, 2, 255, 128}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
	call := jaz.botToolCaller(bot.ID)
	args := map[string]any{"name": name}
	t.Setenv("JAZ_ATTACHMENT_MAX_BYTES", "4")
	call("register_file", args, true)
	t.Setenv("JAZ_ATTACHMENT_MAX_BYTES", "5")
	var file bots.MCPRegisterFileOutput
	decodeMCPTaskResult(t, call("register_file", args, false), &file)
	if file.ID == "" || file.Name != name || file.Size != int64(len(data)) {
		t.Fatalf("registered file = %+v", file)
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
	call("send_message", map[string]any{"attachment_ids": []string{file.ID}}, false)
	events, err := store.LoadSessionEvents(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sent []storage.Attachment
	for _, e := range events {
		if e.RoomMessage != nil && e.RoomMessage.Speaker == "bot" {
			sent = e.RoomMessage.Attachments
		}
	}
	if len(sent) != 1 || sent[0].ID != file.ID || sent[0].SessionID != bot.ID || sent[0].ServerPath != "" {
		t.Fatalf("delivered attachments = %+v", sent)
	}
	jaz.server.AuthKey = "attachment-test-key"
	handler := jaz.server.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sent[0].SessionID+"/attachments/"+sent[0].ID, nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous download returned %d", res.Code)
	}
	req.Header.Set("Authorization", "Bearer attachment-test-key")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	disposition, params, err := mime.ParseMediaType(res.Header().Get("Content-Disposition"))
	if res.Code != http.StatusOK || err != nil || disposition != "attachment" || params["filename"] != name || !bytes.Equal(res.Body.Bytes(), data) {
		t.Fatalf("download returned %d, disposition %q, bytes %x", res.Code, res.Header().Get("Content-Disposition"), res.Body.Bytes())
	}
}

func TestBotFileRegistrationRequiresAnOwnedOutbox(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	jaz.server.Workspace = root
	bot := jaz.newBot("Research")
	s, err := store.LoadSession(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.RuntimeRef.Cwd, ".jaz-outbox")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(s.RuntimeRef.Cwd, "notes.txt")
	if err := os.WriteFile(source, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateSession(storage.CreateSession{Slug: "ordinary", RuntimeRef: s.RuntimeRef})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "local.txt"), []byte("outbox"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.bots.RegisterFile(other.ID, "local.txt"); err == nil {
		t.Fatal("ordinary thread registered a file")
	}
	if err := os.Symlink(source, filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	call := jaz.botToolCaller(bot.ID)
	for _, name := range []string{"../notes.txt", "notes.txt"} {
		call("register_file", map[string]any{"name": name}, true)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(s.RuntimeRef.Cwd, dir); err != nil {
		t.Fatal(err)
	}
	call("register_file", map[string]any{"name": "notes.txt"}, true)
}
